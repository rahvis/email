package batch_mail

import (
	service_batch_mail "billionmail-core/internal/service/batch_mail"
	"billionmail-core/internal/service/contact"
	"billionmail-core/internal/service/kumo"
	"billionmail-core/internal/service/outbound"
	"billionmail-core/internal/service/public"
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"billionmail-core/api/batch_mail/v1"
)

func (c *ControllerV1) ApiMailBatchSend(ctx context.Context, req *v1.ApiMailBatchSendReq) (res *v1.ApiMailBatchSendRes, err error) {
	res = &v1.ApiMailBatchSendRes{}
	clientIP := g.RequestFromCtx(ctx).GetClientIp()

	// 1. check API Key
	apiTemplate, err := getApiTemplateByKey(ctx, req.ApiKey, clientIP)
	if err != nil {
		res.Code = 1001
		res.SetError(gerror.New(public.LangCtx(ctx, err.Error())))
		return res, nil
	}
	if err = validatePublicTenantHeader(ctx, apiTemplate); err != nil {
		res.Code = 1001
		res.SetError(gerror.New(public.LangCtx(ctx, err.Error())))
		return res, nil
	}

	// 2. check client IP
	err = CheckClientIP(ctx, apiTemplate.TenantId, apiTemplate.Id, clientIP)
	if err != nil {
		res.Code = 1002
		res.SetError(gerror.New(public.LangCtx(ctx, err.Error())))
		return res, nil
	}

	// 3. check email template
	_, err = getEmailTemplateById(ctx, apiTemplate.TenantId, apiTemplate.TemplateId)
	if err != nil {
		res.Code = 1004
		res.SetError(gerror.New(public.LangCtx(ctx, "Email template does not exist")))
		return res, nil
	}

	// 4. check recipient
	if len(req.Recipients) == 0 {
		res.Code = 1003
		res.SetError(gerror.New(public.LangCtx(ctx, "Recipients cannot be empty")))
		return res, nil

		//contacts, err := g.DB().Model("bm_contacts").Where("group_id",1).All()
		//if err != nil {
		//	res.Code = 1005
		//	res.SetError(gerror.New(public.LangCtx(ctx, "Failed to get contacts: {}", err.Error())))
		//	return res, nil
		//}
		//recipientaa := []string{}
		//for _, contact := range contacts {
		//	recipientaa = append(recipientaa, contact["email"].String())
		//}
		//req.Recipients = recipientaa
	}

	validRecipients := make([]string, 0, len(req.Recipients))
	for _, recipient := range req.Recipients {
		// remove extra spaces
		recipient = strings.TrimSpace(recipient)
		if recipient != "" && strings.Contains(recipient, "@") {
			validRecipients = append(validRecipients, recipient)
		}
	}
	if len(validRecipients) == 0 {
		res.Code = 1003
		res.SetError(gerror.New(public.LangCtx(ctx, "No valid recipients")))
		return res, nil
	}

	// 5. process addresser
	addresser := req.Addresser
	if addresser == "" {
		addresser = apiTemplate.Addresser
	}
	if err = validateAPISender(ctx, apiTemplate, addresser); err != nil {
		res.Code = 1005
		res.SetError(gerror.New(public.LangCtx(ctx, err.Error())))
		return res, nil
	}
	engine, err := service_batch_mail.ResolveAPIDeliveryEngine(ctx, apiTemplate)
	if err != nil {
		res.Code = 1005
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to resolve delivery engine: {}", err.Error())))
		return res, nil
	}

	batchData := make([]g.Map, 0, len(validRecipients))
	messageIDs := make([]string, 0, len(validRecipients))
	now := int(time.Now().Unix())
	for _, recipient := range validRecipients {

		if apiTemplate.GroupId > 0 {
			if err = validateAPIContactGroup(ctx, apiTemplate.TenantId, apiTemplate.GroupId); err != nil {
				g.Log().Warningf(ctx, "API template %d cannot use group %d: %v", apiTemplate.Id, apiTemplate.GroupId, err)
				res.Code = 1003
				res.SetError(gerror.New(public.LangCtx(ctx, err.Error())))
				return res, nil
			}
			// Add to the specified existing group
			_, err = contact.AddContactToGroup(ctx, recipient, apiTemplate.GroupId)
			if err != nil {
				g.Log().Warningf(ctx, "Failed to add contact %s to group %d: %v", recipient, apiTemplate.GroupId, err)
				continue
			}
		} else {
			// Use the old logic to create an API-specific group
			_, err = ensureContactAndGroup(ctx, recipient, apiTemplate.Id)
			if err != nil {
				g.Log().Warningf(ctx, "Failed to ensure contact and group for %s with API ID %d: %v", recipient, apiTemplate.Id, err)
				continue
			}
		}
		messageId := outbound.GenerateMessageID(addresser)
		messageId = strings.Trim(messageId, "<>")
		batchData = append(batchData, g.Map{
			"api_id":           apiTemplate.Id,
			"tenant_id":        apiTemplate.TenantId,
			"recipient":        recipient,
			"message_id":       messageId,
			"addresser":        addresser,
			"status":           0,
			"engine":           engine,
			"injection_status": "pending",
			"delivery_status":  "pending",
			"attempt_count":    0,
			"next_retry_at":    0,
			"error_message":    "",
			"send_time":        0,
			"create_time":      now,
			"attribs":          req.Attribs,
		})
		messageIDs = append(messageIDs, "<"+messageId+">")
	}

	if len(batchData) == 0 {
		res.Code = 1003
		res.SetError(gerror.New(public.LangCtx(ctx, "No valid recipients to insert")))
		return res, nil
	}

	batchSize := 1000
	for i := 0; i < len(batchData); i += batchSize {
		end := i + batchSize
		if end > len(batchData) {
			end = len(batchData)
		}
		_, err = g.DB().Model("api_mail_logs").Batch(batchSize).Insert(batchData[i:end])
		if err != nil {
			res.Code = 1005
			res.SetError(gerror.New(public.LangCtx(ctx, "Failed to record email log: {}", err.Error())))
			return res, nil
		}
	}

	res.Data = v1.ApiMailBatchSendResult{
		Accepted:        len(batchData),
		Status:          "queued",
		MessageIds:      messageIDs,
		Engine:          engine,
		InjectionStatus: kumo.InjectionStatusPending,
		DeliveryStatus:  kumo.DeliveryStatusPending,
	}
	res.SetSuccess(public.LangCtx(ctx, "Batch email send request accepted, {} emails queued", len(batchData)))
	return res, nil
}
