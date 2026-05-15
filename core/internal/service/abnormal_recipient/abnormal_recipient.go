package abnormal_recipient

import (
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/maillog_stat"
	"billionmail-core/internal/service/tenants"
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func GetListWithPage(ctx context.Context, page, pageSize int, keyword string, addType int) (total int, list []*entity.AbnormalRecipient, err error) {

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	model := tenants.ScopeModel(ctx, g.DB().Model("abnormal_recipient"), "tenant_id").Safe()

	if keyword != "" {
		model = model.WhereLike("recipient", "%"+keyword+"%")
	}
	if addType > 0 {
		model = model.Where("add_type", addType)
	}

	total, err = model.Count()
	if err != nil {
		return 0, nil, fmt.Errorf("Failed to get the total number of exception recipients: %w", err)
	}

	list = make([]*entity.AbnormalRecipient, 0)
	err = model.Page(page, pageSize).
		Order("create_time DESC").
		Scan(&list)

	if err != nil {
		return 0, nil, fmt.Errorf("Failed to get the exception recipient list: %w", err)
	}

	return total, list, nil
}

func Add(ctx context.Context, recipient string) error {

	now := time.Now().Unix()
	_, err := g.DB().Model("abnormal_recipient").
		Data(g.Map{
			"tenant_id":   tenants.CurrentTenantID(ctx),
			"recipient":   recipient,
			"count":       3,
			"add_type":    1,
			"description": "Manually added",
			"create_time": now,
		}).
		InsertIgnore()

	if err != nil {
		return fmt.Errorf("Failed to add exception recipient: %w", err)
	}

	return nil
}

func Delete(ctx context.Context, id int) error {

	_, err := tenants.ScopeModel(ctx, g.DB().Model("abnormal_recipient"), "tenant_id").
		Where("id", id).
		Delete()

	if err != nil {
		return fmt.Errorf("Failed to remove exception recipient: %w", err)
	}

	return nil
}

func GetAbnormalRecipient(ctx context.Context, id int) (*entity.AbnormalRecipient, error) {
	var recipient entity.AbnormalRecipient
	err := tenants.ScopeModel(ctx, g.DB().Model("abnormal_recipient"), "tenant_id").Where("id", id).Scan(&recipient)
	if err != nil {
		return nil, fmt.Errorf("Failed to get exception recipient: %w", err)
	}
	return &recipient, nil
}

func BatchUpsertAbnormalRecipients(ctx context.Context, recipients []string, addType int, description string) error {
	now := time.Now().Unix()

	if len(recipients) == 0 {
		return nil
	}

	var existList []entity.AbnormalRecipient
	err := tenants.ScopeModel(ctx, g.DB().Model("abnormal_recipient"), "tenant_id").WhereIn("recipient", recipients).Scan(&existList)
	if err != nil {
		return fmt.Errorf("Failed to query existing abnormal recipients: %w", err)
	}
	existMap := make(map[string]*entity.AbnormalRecipient)
	for _, r := range existList {
		existMap[r.Recipient] = &r
	}
	// 1. Update the existing one count+1
	for _, r := range existList {
		_, err := tenants.ScopeModel(ctx, g.DB().Model("abnormal_recipient"), "tenant_id").Where("id", r.Id).Data(g.Map{
			"count":       r.Count + 1,
			"description": description,
			"add_type":    addType,
		}).Update()
		if err != nil {

			return fmt.Errorf("Failed to update abnormal recipient: %w", err)
		}

	}
	// 2. Inserting something that doesn't exist
	var insertList []g.Map
	for _, recipient := range recipients {
		if _, ok := existMap[recipient]; !ok {
			insertList = append(insertList, g.Map{
				"tenant_id":   tenants.CurrentTenantID(ctx),
				"recipient":   recipient,
				"count":       1,
				"add_type":    addType,
				"description": description,
				"create_time": now,
			})
		}
	}
	if len(insertList) > 0 {

		_, err := g.DB().Model("abnormal_recipient").Data(insertList).InsertIgnore()
		if err != nil {

			return fmt.Errorf("Failed to insert abnormal recipients: %w", err)
		}
	}

	return nil
}

// BatchUpsertAbnormalRecipientsWithDetails
func BatchUpsertAbnormalRecipientsWithDetails(ctx context.Context, recipientDetails []RecipientDetail, addType int, baseDescription string) error {
	now := time.Now().Unix()

	if len(recipientDetails) == 0 {
		return nil
	}

	recipients := make([]string, len(recipientDetails))
	detailsMap := make(map[string]RecipientDetail)
	for i, detail := range recipientDetails {
		recipients[i] = detail.Email
		detailsMap[detail.Email] = detail
	}

	var existList []entity.AbnormalRecipient
	err := tenants.ScopeModel(ctx, g.DB().Model("abnormal_recipient"), "tenant_id").WhereIn("recipient", recipients).Scan(&existList)
	if err != nil {
		return fmt.Errorf("Failed to query existing abnormal recipients: %w", err)
	}
	existMap := make(map[string]*entity.AbnormalRecipient)
	for _, r := range existList {
		existMap[r.Recipient] = &r
	}

	// 1. Update the existing records
	for _, r := range existList {
		detail := detailsMap[r.Recipient]
		description := fmt.Sprintf("%s - %s", baseDescription, detail.ErrorReason)

		_, err := tenants.ScopeModel(ctx, g.DB().Model("abnormal_recipient"), "tenant_id").Where("id", r.Id).Data(g.Map{
			"count":       r.Count + 1,
			"description": description,
			"add_type":    addType,
		}).Update()
		if err != nil {
			return fmt.Errorf("Failed to update abnormal recipient: %w", err)
		}
	}

	// 2. Inserting a non-existent record
	var insertList []g.Map
	for _, detail := range recipientDetails {
		if _, ok := existMap[detail.Email]; !ok {
			description := fmt.Sprintf("%s - %s", baseDescription, detail.ErrorReason)
			insertList = append(insertList, g.Map{
				"tenant_id":   tenants.CurrentTenantID(ctx),
				"recipient":   detail.Email,
				"count":       1,
				"add_type":    addType,
				"description": description,
				"create_time": now,
			})
		}
	}

	if len(insertList) > 0 {
		_, err := g.DB().Model("abnormal_recipient").Data(insertList).InsertIgnore()
		if err != nil {
			return fmt.Errorf("Failed to insert abnormal recipients: %w", err)
		}
	}

	return nil
}

type RecipientDetail struct {
	Email       string `json:"email"`
	ErrorReason string `json:"error_reason"`
}

func AbnormalRecipientAutoStat(ctx context.Context) {

	var abnormalSwitch string
	val, err := g.DB().Model("bm_options").Where("name", "abnormal_mail_check_switch").Value("value")
	if err == nil && val != nil && val.String() != "" {
		abnormalSwitch = val.String()
	} else {
		abnormalSwitch = "1"
	}
	if abnormalSwitch != "1" {
		return
	}

	lastTime := getLastStatTime(ctx)
	now := time.Now().Unix()

	overview := maillog_stat.NewOverview()
	failedList := overview.FailedListBounced(0, "", lastTime, now)

	recipientDetailsMap := make(map[string]*RecipientDetail)
	for _, item := range failedList {
		recipient, recipientOk := item["recipient"].(string)
		if !recipientOk || recipient == "" {
			continue
		}

		description, descOk := item["description"].(string)
		if !descOk {
			description = "Unknown error"
		}

		if _, exists := recipientDetailsMap[recipient]; !exists {
			recipientDetailsMap[recipient] = &RecipientDetail{
				Email:       recipient,
				ErrorReason: description,
			}
		} else {
			recipientDetailsMap[recipient].ErrorReason = description
		}
	}

	var recipientDetails []RecipientDetail
	for _, detail := range recipientDetailsMap {
		recipientDetails = append(recipientDetails, *detail)
	}

	if len(recipientDetails) > 0 {

		_ = BatchUpsertAbnormalRecipientsWithDetails(ctx, recipientDetails, 2, "Automatic statistics")
	}

	setLastStatTime(ctx, now)

}

func getLastStatTime(ctx context.Context) int64 {
	var lastTime int64
	val, err := g.DB().Model("bm_options").Where("name", "abnormal_recipient_last_time").Value("value")
	if err == nil && val != nil && val.String() != "" {
		lastTime = val.Int64()

	} else {
		lastTime = time.Now().Add(-30 * time.Minute).Unix()

	}
	return lastTime
}

func setLastStatTime(ctx context.Context, t int64) {
	_, err := g.DB().Model("bm_options").
		Data(g.Map{
			"name":  "abnormal_recipient_last_time",
			"value": t,
		}).
		OnConflict("name").
		Save()
	if err != nil {
		g.Log().Debugf(ctx, "[abnormal Recipient] Error occurred in the last statistics time recording: %v", err)
	}
}
