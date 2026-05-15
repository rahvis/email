package v1

import (
	"billionmail-core/utility/types/api_v1"

	"github.com/gogf/gf/v2/frame/g"
)

type ApiTemplates struct {
	Id                int    `json:"id" dc:"id"`
	ApiKey            string `json:"api_key" dc:"api key"`
	ApiKeyHash        string `json:"-" dc:"api key hash"`
	ApiName           string `json:"api_name" dc:"api name"`
	TemplateId        int    `json:"template_id" dc:"template id"`
	TenantId          int    `json:"tenant_id" dc:"tenant id"`
	GroupId           int    `json:"group_id" dc:"group id"`
	Subject           string `json:"subject" dc:"subject"`
	Addresser         string `json:"addresser" dc:"addresser"`
	FullName          string `json:"full_name" dc:"full name"`
	Unsubscribe       int    `json:"unsubscribe" dc:"unsubscribe"`
	TrackOpen         int    `json:"track_open" dc:"track open"`
	TrackClick        int    `json:"track_click" dc:"track click"`
	DeliveryEngine    string `json:"delivery_engine" dc:"delivery engine"`
	SendingProfileId  int    `json:"sending_profile_id" dc:"sending profile id"`
	Active            int    `json:"active" dc:"active"`
	CreateTime        int    `json:"create_time" dc:"create time"`
	UpdateTime        int    `json:"update_time" dc:"update time"`
	ExpireTime        int    `json:"expire_time" dc:"expire time"`
	LastKeyUpdateTime int    `json:"last_key_update_time" dc:"last key update time"`
}

type ApiMailLogs struct {
	Id                   int    `json:"id" dc:"id"`
	ApiId                int    `json:"api_id" dc:"api id"`
	TenantId             int    `json:"tenant_id" dc:"tenant id"`
	Recipient            string `json:"recipient" dc:"recipient"`
	MessageId            string `json:"message_id" dc:"message id"`
	Addresser            string `json:"addresser" dc:"addresser"`
	Engine               string `json:"engine" dc:"outbound engine"`
	InjectionStatus      string `json:"injection_status" dc:"injection status"`
	DeliveryStatus       string `json:"delivery_status" dc:"delivery status"`
	KumoQueue            string `json:"kumo_queue" dc:"kumo queue"`
	ProviderQueueId      string `json:"provider_queue_id" dc:"provider queue id"`
	LastDeliveryEventAt  int    `json:"last_delivery_event_at" dc:"last delivery event"`
	LastDeliveryResponse string `json:"last_delivery_response" dc:"last delivery response"`
	AttemptCount         int    `json:"attempt_count" dc:"attempt count"`
	NextRetryAt          int    `json:"next_retry_at" dc:"next retry at"`
	SendTime             int    `json:"send_time" dc:"send time"`
}

type ApiTemplatesListReq struct {
	g.Meta        `path:"/batch_mail/api/list" method:"get" tags:"ApiMail" summary:"api list"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	Page          int    `json:"page" dc:"page"`
	PageSize      int    `json:"page_size" dc:"page size"`
	Keyword       string `json:"keyword" dc:"Search Keyword"`
	Active        int    `json:"active" dc:"active"`
	StartTime     int    `json:"start_time" dc:"start time"`
	EndTime       int    `json:"end_time" dc:"end time"`
}

type ApiTemplatesInfo struct {
	ApiTemplates
	GroupId           int     `json:"group_id" dc:"group id"`
	SendCount         int     `json:"send_count" dc:"send count"`
	SuccessCount      int     `json:"success_count" dc:"success count"`
	FailCount         int     `json:"fail_count" dc:"fail count"`
	QueuedCount       int     `json:"queued_count" dc:"queued count"`
	DeliveredCount    int     `json:"delivered_count" dc:"delivered count"`
	DeferredCount     int     `json:"deferred_count" dc:"deferred count"`
	BouncedCount      int     `json:"bounced_count" dc:"bounced count"`
	ExpiredCount      int     `json:"expired_count" dc:"expired count"`
	ComplainedCount   int     `json:"complained_count" dc:"complained count"`
	WebhookLastSeenAt int64   `json:"webhook_last_seen_at" dc:"webhook last seen"`
	WebhookLagSeconds int64   `json:"webhook_lag_seconds" dc:"webhook lag seconds"`
	WebhookHealthy    bool    `json:"webhook_healthy" dc:"webhook healthy"`
	OpenRate          float64 `json:"open_rate" dc:"open rate"`
	ClickRate         float64 `json:"click_rate" dc:"click rate"`
	DeliveryRate      float64 `json:"delivery_rate" dc:"delivery rate"`
	BounceRate        float64 `json:"bounce_rate" dc:"bounce rate"`
	//UnsubscribeCount int      `json:"unsubscribe_count" dc:"unsubscribe count"`
	IpWhitelist     []string `json:"ip_whitelist" dc:"IP whitelist"`
	ServerAddresser string   `json:"server_addresser" dc:"server addresser"`
}

type ApiTemplatesListRes struct {
	api_v1.StandardRes
	Data struct {
		Total int                 `json:"total" dc:"total"`
		List  []*ApiTemplatesInfo `json:"list"  dc:"api templates list"`
	} `json:"data"`
}

type ApiSummaryStats struct {
	TotalSend       int     `json:"total_send" dc:"total send count"`
	AvgDeliveryRate float64 `json:"avg_delivery_rate" dc:"average delivery rate"`
	AvgOpenRate     float64 `json:"avg_open_rate" dc:"average open rate"`
	AvgClickRate    float64 `json:"avg_click_rate" dc:"average click rate"`
	AvgBounceRate   float64 `json:"avg_bounce_rate" dc:"average bounce rate"`
	//AvgUnsubRate     float64 `json:"avg_unsub_rate" dc:"average unsubscribe rate"`
	//TotalUnsubscribe int     `json:"total_unsubscribe" dc:"total unsubscribe count"`
}

type ApiOverviewStatsReq struct {
	g.Meta        `path:"/batch_mail/api/overview_stats" method:"get" tags:"ApiMail" summary:"api Overview"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	StartTime     int    `json:"start_time" dc:"start time"`
	EndTime       int    `json:"end_time" dc:"end time"`
}

type ApiOverviewStatsRes struct {
	api_v1.StandardRes
	Data ApiSummaryStats `json:"data"`
}

type ApiTemplatesCreateReq struct {
	g.Meta        `path:"/batch_mail/api/create" method:"post" tags:"ApiMail" summary:"api create"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	ApiName       string `json:"api_name" dc:"api name"`
	TemplateId    int    `json:"template_id" dc:"template id"`
	GroupId       int    `json:"group_id" dc:"Associated group ID"`
	Subject       string `json:"subject" dc:"subject"`
	Addresser     string `json:"addresser" dc:"addresser"`
	FullName      string `json:"full_name" dc:"full name"`
	Unsubscribe   int    `json:"unsubscribe" dc:"unsubscribe"`
	Active        int    `json:"active" dc:"active"`
	ExpireTime    int    `json:"expire_time" dc:"expire time"` // 0 is a permanently valid unit of seconds
	//IpWhitelistEnabled int      `json:"ip_whitelist_enabled" dc:"ip whitelist enabled"`
	IpWhitelist      []string `json:"ip_whitelist" dc:"ip whitelist"`
	TrackOpen        int      `json:"track_open" v:"in:0,1" dc:"track open" default:"1"`
	TrackClick       int      `json:"track_click" v:"in:0,1" dc:"track click" default:"1"`
	DeliveryEngine   string   `json:"delivery_engine" v:"in:tenant_default,kumomta,postfix,local,smtp,kumo,inherited,default,inherit" dc:"delivery engine" default:"postfix"`
	SendingProfileId int      `json:"sending_profile_id" v:"min:0" dc:"sending profile id" default:"0"`
}

type ApiTemplatesCreateRes struct {
	api_v1.StandardRes
}

type ApiTemplatesUpdateReq struct {
	g.Meta        `path:"/batch_mail/api/update" method:"post" tags:"ApiMail" summary:"api update"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	ID            int    `json:"id" dc:"id"`
	ApiName       string `json:"api_name" dc:"api name"`
	TemplateId    int    `json:"template_id" dc:"template id"`
	GroupId       int    `json:"group_id" dc:"Associated group ID"`
	Subject       string `json:"subject" dc:"subject"`
	Addresser     string `json:"addresser" dc:"addresser"`
	FullName      string `json:"full_name" dc:"full name"`
	Unsubscribe   int    `json:"unsubscribe" dc:"unsubscribe"`
	TrackOpen     int    `json:"track_open" dc:"track open"`
	TrackClick    int    `json:"track_click" dc:"track click"`
	Active        int    `json:"active" dc:"active"`
	ExpireTime    int    `json:"expire_time" dc:"Key expiration time (0 is permanent)"`
	ResetKey      bool   `json:"reset_key" dc:"reset key"`
	//IpWhitelistEnabled int      `json:"ip_whitelist_enabled" dc:"ip whitelist enabled"`
	IpWhitelist      []string `json:"ip_whitelist" dc:"ip whitelist"`
	DeliveryEngine   string   `json:"delivery_engine" v:"in:tenant_default,kumomta,postfix,local,smtp,kumo,inherited,default,inherit" dc:"delivery engine"`
	SendingProfileId int      `json:"sending_profile_id" v:"min:0" dc:"sending profile id"`
}

type ApiTemplatesUpdateRes struct {
	api_v1.StandardRes
}

type ApiTemplatesDeleteReq struct {
	g.Meta        `path:"/batch_mail/api/delete" method:"post" tags:"ApiMail" summary:"api delete"`
	Authorization string `json:"authorization" dc:"Authorization" in:"header"`
	ID            int    `json:"id" dc:"id"`
}

type ApiTemplatesDeleteRes struct {
	api_v1.StandardRes
}

type ApiMailSendReq struct {
	g.Meta        `path:"/batch_mail/api/send" method:"post" tags:"ApiMail" summary:"call api send mail"`
	Authorization string            `json:"authorization" dc:"Authorization" in:"header"`
	ApiKey        string            `json:"x-api-key" dc:"API Key" in:"header"`
	Addresser     string            `json:"addresser" dc:"addresser"`
	Recipient     string            `json:"recipient" dc:"recipient"`
	Attribs       map[string]string `json:"attribs" dc:"Custom properties"`
}

type ApiMailSendRes struct {
	api_v1.StandardRes
	Data ApiMailSendResult `json:"data"`
}

type ApiMailSendResult struct {
	Accepted        bool   `json:"accepted" dc:"accepted"`
	Status          string `json:"status" dc:"queued status"`
	ApiLogId        int64  `json:"api_log_id" dc:"api mail log id"`
	MessageId       string `json:"message_id" dc:"BillionMail Message-ID"`
	Engine          string `json:"engine" dc:"delivery engine"`
	InjectionStatus string `json:"injection_status" dc:"injection status"`
	DeliveryStatus  string `json:"delivery_status" dc:"delivery status"`
}

type ApiMailBatchSendReq struct {
	g.Meta        `path:"/batch_mail/api/batch_send" method:"post" tags:"ApiMail" summary:"call api batch send mail"`
	Authorization string            `json:"authorization" dc:"Authorization" in:"header"`
	ApiKey        string            `json:"x-api-key" dc:"API Key" in:"header"`
	Addresser     string            `json:"addresser" dc:"addresser"`
	Recipients    []string          `json:"recipients" dc:"recipients"`
	Attribs       map[string]string `json:"attribs" dc:"Custom properties"`
}

type ApiMailBatchSendRes struct {
	api_v1.StandardRes
	Data ApiMailBatchSendResult `json:"data"`
}

type ApiMailBatchSendResult struct {
	Accepted        int      `json:"accepted" dc:"accepted count"`
	Status          string   `json:"status" dc:"queued status"`
	MessageIds      []string `json:"message_ids" dc:"BillionMail Message-IDs"`
	Engine          string   `json:"engine" dc:"delivery engine"`
	InjectionStatus string   `json:"injection_status" dc:"injection status"`
	DeliveryStatus  string   `json:"delivery_status" dc:"delivery status"`
}
