package outbound

import (
	"fmt"
	"net/mail"
	"sort"
	"strconv"
	"strings"
)

const unknownDestinationDomain = "unknown.local"

func DestinationDomain(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return unknownDestinationDomain
	}
	if parsed, err := mail.ParseAddress(address); err == nil {
		address = parsed.Address
	}
	parts := strings.Split(address, "@")
	if len(parts) != 2 {
		return unknownDestinationDomain
	}
	domain := strings.ToLower(strings.Trim(parts[1], " \t\r\n."))
	if domain == "" {
		return unknownDestinationDomain
	}
	return domain
}

func SenderDomain(address string) string {
	return DestinationDomain(address)
}

func QueueNameForCampaign(campaignID, tenantID int64, destinationDomain string) string {
	return queueName(fmt.Sprintf("campaign_%d", campaignID), tenantID, destinationDomain)
}

func QueueNameForAPI(apiID, tenantID int64, destinationDomain string) string {
	return queueName(fmt.Sprintf("api_%d", apiID), tenantID, destinationDomain)
}

func QueueNameForMessage(req OutboundMessage) string {
	domain := req.DestinationDomain
	if domain == "" {
		domain = DestinationDomain(req.Recipient)
	}
	if req.APIID > 0 {
		return QueueNameForAPI(req.APIID, req.TenantID, domain)
	}
	if req.CampaignID > 0 {
		return QueueNameForCampaign(req.CampaignID, req.TenantID, domain)
	}
	return queueName("system", req.TenantID, domain)
}

func CorrelationHeaders(req OutboundMessage) map[string]string {
	headers := map[string]string{
		"X-BM-Tenant-ID":          strconv.FormatInt(req.TenantID, 10),
		"X-BM-Campaign-ID":        strconv.FormatInt(req.CampaignID, 10),
		"X-BM-Task-ID":            strconv.FormatInt(req.TaskID, 10),
		"X-BM-Recipient-ID":       strconv.FormatInt(req.RecipientID, 10),
		"X-BM-Api-ID":             strconv.FormatInt(req.APIID, 10),
		"X-BM-Api-Log-ID":         strconv.FormatInt(req.APILogID, 10),
		"X-BM-Message-ID":         req.MessageID,
		"X-BM-Sending-Profile-ID": strconv.FormatInt(req.SendingProfileID, 10),
		"X-BM-Engine":             EngineKumoMTA,
	}
	for key, value := range headers {
		if value == "" {
			delete(headers, key)
		}
	}
	return headers
}

func RecipientMetadata(req OutboundMessage, queueName string) map[string]string {
	meta := map[string]string{}
	for key, value := range req.Metadata {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		meta[key] = value
	}
	if queueName != "" {
		meta["queue"] = queueName
	}
	if req.TenantID > 0 {
		meta["tenant"] = fmt.Sprintf("tenant_%d", req.TenantID)
	}
	if req.CampaignID > 0 {
		meta["campaign"] = fmt.Sprintf("campaign_%d", req.CampaignID)
	} else if req.APIID > 0 {
		meta["campaign"] = fmt.Sprintf("api_%d", req.APIID)
	} else {
		meta["campaign"] = "system"
	}
	for key, value := range CorrelationHeaders(req) {
		meta[metadataKey(key)] = value
	}
	return meta
}

func SortedHeaderKeys(headers map[string]string) []string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func queueName(campaign string, tenantID int64, destinationDomain string) string {
	domain := DestinationDomain(destinationDomain)
	if destinationDomain != "" && !strings.Contains(destinationDomain, "@") {
		domain = strings.ToLower(strings.Trim(destinationDomain, " \t\r\n."))
	}
	if domain == "" {
		domain = unknownDestinationDomain
	}
	tenant := "tenant_0"
	if tenantID > 0 {
		tenant = fmt.Sprintf("tenant_%d", tenantID)
	}
	return fmt.Sprintf("%s:%s@%s", campaign, tenant, domain)
}

func metadataKey(header string) string {
	header = strings.ToLower(strings.TrimSpace(header))
	header = strings.TrimPrefix(header, "x-")
	header = strings.ReplaceAll(header, "-", "_")
	return header
}
