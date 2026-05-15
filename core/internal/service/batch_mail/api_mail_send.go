// core/internal/service/batch_mail/api_mail_send.go
package batch_mail

import (
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/domains"
	"billionmail-core/internal/service/public"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/util/guid"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	StatusPending = 0
	StatusSuccess = 2
	StatusFailed  = 3

	BatchSize    = 1000 // Number of items to process per batch
	WorkerCount  = 20   // Number of worker goroutines
	QueryTimeout = 15   // Query timeout duration (seconds)
	SendTimeout  = 15   // Send timeout duration (seconds)
	LockKey      = "api_mail_queue_lock"
	LockTimeout  = 60 * 2 // Lock timeout duration (seconds)
)

const (
	renewAPIMailQueueLockScript   = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("EXPIRE", KEYS[1], ARGV[2]) else return 0 end`
	releaseAPIMailQueueLockScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
)

var (
	acquireAPIMailQueueLock = acquireRedisAPIMailQueueLock
	renewAPIMailQueueLock   = renewRedisAPIMailQueueLock
	releaseAPIMailQueueLock = releaseRedisAPIMailQueueLock
	processAPIMailQueue     = ProcessApiMailQueue
)

type ApiMailLog struct {
	Id           int64
	ApiId        int
	TenantId     int64
	Recipient    string
	Addresser    string
	MessageId    string
	Engine       string
	AttemptCount int
	NextRetryAt  int64
	Attribs      map[string]string `json:"attribs"`
}

// Cache data structure
type CacheData struct {
	ApiTemplates   map[int]entity.ApiTemplates
	EmailTemplates map[int]entity.EmailTemplate
	Contacts       map[string]entity.Contact
}

// Worker pool structure
type WorkerPool struct {
	workers int
	jobs    chan ApiMailLog
	wg      sync.WaitGroup
	cache   *CacheData
	//rateLimiter <-chan time.Time
}

// Create a new worker pool
//func NewWorkerPool(workers int, cache *CacheData, ratePerMinute int) *WorkerPool {
//	//interval := time.Minute / time.Duration(ratePerMinute)
//	return &WorkerPool{
//		workers: workers,
//		jobs:    make(chan ApiMailLog, BatchSize),
//		cache:   cache,
//		//rateLimiter: time.Tick(interval), // The interval between each email
//	}
//}

// Start the worker pool
func (p *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(ctx)
	}
}

// Worker goroutine
func (p *WorkerPool) worker(ctx context.Context) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case log, ok := <-p.jobs:
			if !ok {
				return
			}
			p.processMail(ctx, log)
		}
	}
}

// Process a single email
func (p *WorkerPool) processMail(ctx context.Context, log ApiMailLog) {
	ctx, cancel := context.WithTimeout(ctx, SendTimeout*time.Second)
	defer cancel()

	// Get data from cache, if not exist, fetch from database
	apiTemplate, ok := p.cache.ApiTemplates[log.ApiId]
	if !ok {
		updateLogStatus(ctx, log, StatusFailed, fmt.Sprintf("Failed to get API template: %d", log.ApiId))
		return
	}

	emailTemplate, ok := p.cache.EmailTemplates[apiTemplate.TemplateId]
	if !ok {
		updateLogStatus(ctx, log, StatusFailed, fmt.Sprintf("Failed to get email template: %d", apiTemplate.TemplateId))
		return
	}

	contact, ok := p.cache.Contacts[tenantContactKey(log.TenantId, log.Recipient)]
	if !ok {
		contact = entity.Contact{Email: log.Recipient}
	}

	// Process email content and subject
	content, subject := processMailContentAndSubject(ctx, emailTemplate.Content, apiTemplate.Subject, &apiTemplate, contact, log)

	result := sendApiMailPrepared(ctx, &apiTemplate, subject, content, log)
	if err := applyAPIMailSendResult(ctx, log, result); err != nil {
		g.Log().Errorf(ctx, "Failed to update API mail log %d: %v", log.Id, err)
		return
	}
}

// Queue processing with distributed lock
func ProcessApiMailQueueWithLock(ctx context.Context) {
	ownerToken, locked, err := acquireAPIMailQueueLock(ctx)
	if err != nil {
		g.Log().Error(ctx, "Failed to acquire lock:", err)
		return
	}
	if !locked {
		g.Log().Debug(ctx, "Another instance is processing the API mail queue")
		return
	}

	// Start the goroutine for lease renewal
	stopRenew := make(chan struct{})
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if renewed, err := renewAPIMailQueueLock(ctx, ownerToken); err != nil {
					g.Log().Warning(ctx, "Failed to renew API mail queue lock:", err)
				} else if !renewed {
					g.Log().Warning(ctx, "API mail queue lock ownership lost during renewal")
				}
			case <-stopRenew:
				return
			}
		}
	}()

	defer func() {
		close(stopRenew)
		if err := releaseAPIMailQueueLock(ctx, ownerToken); err != nil {
			g.Log().Warning(ctx, "Failed to release API mail queue lock:", err)
		}
	}()

	// Process the email queue
	processAPIMailQueue(ctx)
}

func acquireRedisAPIMailQueueLock(ctx context.Context) (string, bool, error) {
	ownerToken := guid.S()
	result, err := g.Redis().Do(ctx, "SET", LockKey, ownerToken, "EX", LockTimeout, "NX")
	if err != nil {
		return "", false, err
	}
	if result == nil || result.IsNil() || strings.ToUpper(result.String()) != "OK" {
		return "", false, nil
	}
	return ownerToken, true, nil
}

func renewRedisAPIMailQueueLock(ctx context.Context, ownerToken string) (bool, error) {
	result, err := g.Redis().Do(ctx, "EVAL", renewAPIMailQueueLockScript, 1, LockKey, ownerToken, LockTimeout)
	if err != nil {
		return false, err
	}
	return result != nil && result.Int() == 1, nil
}

func releaseRedisAPIMailQueueLock(ctx context.Context, ownerToken string) error {
	_, err := g.Redis().Do(ctx, "EVAL", releaseAPIMailQueueLockScript, 1, LockKey, ownerToken)
	return err
}

// Batch process emails
func ProcessApiMailQueue(ctx context.Context) {
	// Process all pending emails in batches
	lastMaxId := int64(0)
	for {
		// Get a batch of pending emails
		var mailLogs []ApiMailLog
		queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout*time.Second)
		err := g.DB().Model("api_mail_logs").
			Where("status = ?", StatusPending).
			Where("(next_retry_at = 0 OR next_retry_at <= ?)", time.Now().Unix()).
			Where("id > ?", lastMaxId).
			Order("id ASC").
			Limit(BatchSize).
			Ctx(queryCtx).
			Scan(&mailLogs)
		cancel()

		if err != nil || len(mailLogs) == 0 {
			break
		}

		// Preload data
		cache, err := preloadData(ctx, mailLogs)
		if err != nil {
			g.Log().Error(ctx, "Failed to preload data:", err)
			break
		}

		pool := &WorkerPool{
			workers: WorkerCount,
			jobs:    make(chan ApiMailLog, len(mailLogs)),
			cache:   cache,
		}
		pool.Start(ctx)
		for _, log := range mailLogs {
			pool.jobs <- log
		}
		close(pool.jobs)
		pool.wg.Wait()

		// lastMaxId = mailLogs[len(mailLogs)-1].Id
		if len(mailLogs) > 0 {
			lastMaxId = mailLogs[len(mailLogs)-1].Id
		}
		// offset += len(mailLogs)
		if len(mailLogs) < BatchSize {
			break
		}
	}
}

func preloadData(ctx context.Context, logs []ApiMailLog) (*CacheData, error) {
	// Collect all necessary IDs
	apiIds := make([]int, 0, len(logs))
	recipientEmails := make([]string, 0, len(logs))
	tenantIds := make([]int64, 0, len(logs))
	tenantSeen := make(map[int64]struct{})
	for _, log := range logs {
		apiIds = append(apiIds, log.ApiId)
		recipientEmails = append(recipientEmails, log.Recipient)
		if _, ok := tenantSeen[log.TenantId]; !ok {
			tenantSeen[log.TenantId] = struct{}{}
			tenantIds = append(tenantIds, log.TenantId)
		}
	}
	// Batch query API templates
	var apiTemplates []entity.ApiTemplates
	err := g.DB().Model("api_templates").
		WhereIn("id", apiIds).
		Ctx(ctx).
		Scan(&apiTemplates)
	if err != nil {
		return nil, fmt.Errorf("failed to load API templates: %v", err)
	}
	// Collect template IDs
	templateIds := make([]int, 0, len(apiTemplates))
	for _, t := range apiTemplates {
		templateIds = append(templateIds, t.TemplateId)
	}
	// Batch query email templates
	var emailTemplates []entity.EmailTemplate
	err = g.DB().Model("email_templates").
		WhereIn("id", templateIds).
		Ctx(ctx).
		Scan(&emailTemplates)
	if err != nil {
		return nil, fmt.Errorf("failed to load email templates: %v", err)
	}
	// Batch query contacts
	var contacts []entity.Contact
	err = g.DB().Model("bm_contacts").
		WhereIn("tenant_id", tenantIds).
		WhereIn("email", recipientEmails).
		Ctx(ctx).
		Scan(&contacts)
	if err != nil {
		return nil, fmt.Errorf("failed to load contacts: %v", err)
	}
	// Build cache
	cache := &CacheData{
		ApiTemplates:   make(map[int]entity.ApiTemplates),
		EmailTemplates: make(map[int]entity.EmailTemplate),
		Contacts:       make(map[string]entity.Contact),
	}

	for _, t := range apiTemplates {
		cache.ApiTemplates[t.Id] = t
	}
	for _, t := range emailTemplates {
		cache.EmailTemplates[t.Id] = t
	}
	for _, c := range contacts {
		cache.Contacts[tenantContactKey(int64(c.TenantId), c.Email)] = c
	}

	return cache, nil
}

func tenantContactKey(tenantID int64, email string) string {
	return fmt.Sprintf("%d:%s", tenantID, strings.ToLower(strings.TrimSpace(email)))
}

func updateLogStatus(ctx context.Context, log ApiMailLog, status int, errorMsg string) {
	result := apiMailSendResult{Status: status, ErrorMessage: errorMsg}
	switch status {
	case StatusSuccess:
		result.Engine = APIDeliveryEnginePostfix
		result.InjectionStatus = "queued"
		result.DeliveryStatus = "pending"
	case StatusFailed:
		result.Engine = APIDeliveryEnginePostfix
		result.InjectionStatus = "failed"
		result.DeliveryStatus = "unknown"
	}
	if err := applyAPIMailSendResult(ctx, log, result); err != nil {
		g.Log().Error(ctx, "Failed to update mail log status:", err)
	}
}

// process mail content and subject
func processMailContentAndSubject(ctx context.Context, content, subject string, apiTemplate *entity.ApiTemplates, contact entity.Contact, log ApiMailLog) (string, string) {
	// unsubscribe link processing

	apiAttribs := make(map[string]interface{})
	if log.Attribs != nil {
		for k, v := range log.Attribs {
			apiAttribs[k] = v
		}
	}

	if apiTemplate.Unsubscribe == 1 {
		if !strings.Contains(content, "__UNSUBSCRIBE_URL__") && !strings.Contains(content, "{{ UnsubscribeURL . }}") {
			content = public.AddUnsubscribeButton(content)
		}

		domain := domains.GetBaseURLBySender(log.Addresser)

		jwtToken, _ := GenerateUnsubscribeJWT(log.Recipient, apiTemplate.TemplateId, apiTemplate.Id, contact.GroupId)
		var unsubscribeJumpURL string
		if contact.GroupId > 0 {
			unsubscribeJumpURL = fmt.Sprintf("%s/unsubscribe_new.html?jwt=%s",
				domain, jwtToken)

		} else {
			unsubscribeURL := fmt.Sprintf("%s/api/unsubscribe", domain)
			groupURL := fmt.Sprintf("%s/api/unsubscribe/user_group", domain)
			unsubscribeJumpURL = fmt.Sprintf("%s/unsubscribe.html?jwt=%s&email=%s&url_type=%s&url_unsubscribe=%s", domain, jwtToken, log.Recipient, groupURL, unsubscribeURL)
		}

		if contact.Id > 0 {
			engine := GetTemplateEngine()
			renderedContent, err := engine.RenderEmailTemplateWithAPI(ctx, content, &contact, nil, unsubscribeJumpURL, apiAttribs)
			if err == nil {
				content = renderedContent
			}
			renderedSubject, err := engine.RenderEmailTemplateWithAPI(ctx, subject, &contact, nil, unsubscribeJumpURL, apiAttribs)
			if err == nil {
				subject = renderedSubject
			}
		} else {
			content = strings.ReplaceAll(content, "{{ UnsubscribeURL . }}", unsubscribeJumpURL)
		}
	} else if contact.Id > 0 {
		engine := GetTemplateEngine()
		renderedContent, err := engine.RenderEmailTemplateWithAPI(ctx, content, &contact, nil, "", apiAttribs)
		if err == nil {
			content = renderedContent
		}
		renderedSubject, err := engine.RenderEmailTemplateWithAPI(ctx, subject, &contact, nil, "", apiAttribs)
		if err == nil {
			subject = renderedSubject
		}
	}
	return content, subject
}
