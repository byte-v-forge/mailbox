package main

import (
	"context"
	"fmt"
	"github.com/byte-v-forge/common-lib/stringx"
	"net/http"
	"os"
	"strings"
	"time"

	cloudflare "github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/dns"
	"github.com/cloudflare/cloudflare-go/v7/email_routing"
	"github.com/cloudflare/cloudflare-go/v7/option"
	"github.com/cloudflare/cloudflare-go/v7/zones"

	"github.com/byte-v-forge/common-lib/envx"
	"github.com/byte-v-forge/common-lib/protojsonx"

	"mailboxapi/pb"
)

const defaultCloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"

func loadCloudflareEmailDomains() []string {
	cfg := loadCloudflareEmailConfig()
	token := strings.TrimSpace(os.Getenv("MAILBOX_CLOUDFLARE_API_TOKEN"))
	if token == "" {
		if cfg != nil && len(cfg.GetZones()) > 0 {
			logWarning("MAILBOX_CLOUDFLARE_API_TOKEN is required to load Cloudflare email domains")
		}
		return nil
	}
	timeout := time.Duration(envx.Int("MAILBOX_CLOUDFLARE_API_TIMEOUT_SECONDS", defaultHTTPTimeoutSeconds)) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	domains, err := fetchCloudflareEmailDomains(ctx, &http.Client{Timeout: timeout}, token, cfg)
	if err != nil {
		logWarning("fetch Cloudflare email config: %v", err)
		return nil
	}
	if len(domains) == 0 {
		logWarning("Cloudflare email API returned no mailbox domains")
		return nil
	}
	logInfo("loaded Cloudflare email domains from API count=%d", len(domains))
	return domains
}

func loadCloudflareEmailConfig() *pb.CloudflareEmailConfig {
	path := strings.TrimSpace(os.Getenv("MAILBOX_CLOUDFLARE_EMAIL_CONFIG_FILE"))
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		logWarning("read Cloudflare email config: %v", err)
		return nil
	}
	var cfg pb.CloudflareEmailConfig
	if err := protojsonx.Unmarshal(raw, &cfg); err != nil {
		logWarning("decode Cloudflare email config: %v", err)
		return nil
	}
	return &cfg
}

func fetchCloudflareEmailDomains(ctx context.Context, httpClient *http.Client, token string, cfg *pb.CloudflareEmailConfig) ([]string, error) {
	if cfg == nil {
		cfg = &pb.CloudflareEmailConfig{}
	}
	api := newCloudflareEmailAPI(httpClient, token, strings.TrimRight(envx.StringDefault("MAILBOX_CLOUDFLARE_API_BASE_URL", cfg.GetApiBaseUrl()), "/"))
	out := []string{}
	seen := map[string]struct{}{}
	zones, err := api.cloudflareEmailZones(ctx, cfg.GetZones())
	if err != nil {
		return nil, err
	}
	for _, zone := range zones {
		if !optionalBoolEnabled(zone.Enabled) || !optionalBoolEnabled(zone.CatchAllEnabled) {
			continue
		}
		zoneID, zoneName, err := api.resolveZone(ctx, zone)
		if err != nil {
			logWarning("skip Cloudflare email zone %s: %v", cloudflareZoneLabel(zone), err)
			continue
		}
		catchAll, err := api.emailRoutingCatchAll(ctx, zoneID)
		if err != nil {
			logWarning("skip Cloudflare email zone %s catch-all: %v", stringx.FirstNonEmpty(zoneName, cloudflareZoneLabel(zone)), err)
			continue
		}
		if !cloudflareCatchAllWorkerEnabled(catchAll, strings.TrimSpace(zone.GetWorkerName())) {
			continue
		}
		mxDomains, err := api.emailRoutingMXDomains(ctx, zoneID)
		if err != nil {
			logWarning("skip Cloudflare email zone %s MX: %v", stringx.FirstNonEmpty(zoneName, cloudflareZoneLabel(zone)), err)
			continue
		}
		added := false
		for _, domain := range mxDomains {
			added = appendCloudflareEmailDomain(&out, seen, domain) || added
		}
		if !added {
			appendCloudflareEmailDomain(&out, seen, zoneName)
		}
	}
	return out, nil
}

type cloudflareEmailAPI struct {
	client *cloudflare.Client
}

func newCloudflareEmailAPI(httpClient *http.Client, token string, baseURL string) *cloudflareEmailAPI {
	if baseURL == "" {
		baseURL = defaultCloudflareAPIBaseURL
	}
	return &cloudflareEmailAPI{client: cloudflare.NewClient(
		option.WithAPIToken(token),
		option.WithHTTPClient(httpClient),
		option.WithBaseURL(baseURL),
		option.WithMaxRetries(0),
	)}
}

func (api *cloudflareEmailAPI) resolveZone(ctx context.Context, zone *pb.CloudflareEmailZone) (string, string, error) {
	zoneID := strings.TrimSpace(zone.GetZoneId())
	zoneName := normalizeCloudflareDomain(zone.GetZoneName())
	if zoneID != "" {
		return zoneID, zoneName, nil
	}
	if zoneName == "" {
		return "", "", fmt.Errorf("Cloudflare email zone requires zone_id or zone_name")
	}
	iter := api.client.Zones.ListAutoPaging(ctx, zones.ZoneListParams{
		Name:    cloudflare.F(zoneName),
		PerPage: cloudflare.F(float64(1)),
	})
	if iter.Next() {
		found := iter.Current()
		return found.ID, normalizeCloudflareDomain(found.Name), nil
	}
	if err := iter.Err(); err != nil {
		return "", "", err
	}
	return "", "", fmt.Errorf("Cloudflare zone not found: %s", zoneName)
}

func (api *cloudflareEmailAPI) cloudflareEmailZones(ctx context.Context, configured []*pb.CloudflareEmailZone) ([]*pb.CloudflareEmailZone, error) {
	out := []*pb.CloudflareEmailZone{}
	seen := map[string]struct{}{}
	add := func(zone *pb.CloudflareEmailZone) {
		if zone == nil {
			return
		}
		key := stringx.FirstNonEmpty(strings.TrimSpace(zone.GetZoneId()), normalizeCloudflareDomain(zone.GetZoneName()))
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, zone)
	}
	for _, zone := range configured {
		add(zone)
	}

	iter := api.client.Zones.ListAutoPaging(ctx, zones.ZoneListParams{PerPage: cloudflare.F(float64(50))})
	for iter.Next() {
		zone := iter.Current()
		name := normalizeCloudflareDomain(zone.Name)
		if zone.ID == "" || name == "" {
			continue
		}
		add(&pb.CloudflareEmailZone{ZoneId: zone.ID, ZoneName: name})
	}
	if err := iter.Err(); err != nil {
		if len(out) > 0 {
			logWarning("skip dynamic Cloudflare zone discovery: %v", err)
			return out, nil
		}
		return nil, err
	}
	return out, nil
}

func (api *cloudflareEmailAPI) emailRoutingCatchAll(ctx context.Context, zoneID string) (*email_routing.RuleCatchAllGetResponse, error) {
	return api.client.EmailRouting.Rules.CatchAlls.Get(ctx, email_routing.RuleCatchAllGetParams{ZoneID: cloudflare.F(zoneID)})
}

func (api *cloudflareEmailAPI) emailRoutingMXDomains(ctx context.Context, zoneID string) ([]string, error) {
	iter := api.client.DNS.Records.ListAutoPaging(ctx, dns.RecordListParams{
		ZoneID:  cloudflare.F(zoneID),
		Type:    cloudflare.F(dns.RecordListParamsTypeMX),
		PerPage: cloudflare.F(float64(100)),
	})
	out := []string{}
	for iter.Next() {
		record := iter.Current()
		content := strings.ToLower(strings.TrimSpace(record.Content))
		if !strings.HasSuffix(content, ".mx.cloudflare.net") {
			continue
		}
		out = append(out, record.Name)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func cloudflareCatchAllWorkerEnabled(rule *email_routing.RuleCatchAllGetResponse, workerName string) bool {
	if rule == nil || !bool(rule.Enabled) {
		return false
	}
	for _, action := range rule.Actions {
		if action.Type != email_routing.CatchAllActionTypeWorker && strings.TrimSpace(strings.ToLower(string(action.Type))) != "worker" {
			continue
		}
		if workerName == "" {
			return true
		}
		for _, value := range action.Value {
			if strings.EqualFold(strings.TrimSpace(value), workerName) {
				return true
			}
		}
	}
	return false
}

func optionalBoolEnabled(value *bool) bool {
	return value == nil || *value
}

func appendCloudflareEmailDomain(out *[]string, seen map[string]struct{}, value string) bool {
	domain := normalizeCloudflareDomain(value)
	if domain == "" {
		return false
	}
	if _, ok := seen[domain]; ok {
		return false
	}
	seen[domain] = struct{}{}
	*out = append(*out, domain)
	return true
}

func normalizeCloudflareDomain(value string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".")
}

func cloudflareZoneLabel(zone *pb.CloudflareEmailZone) string {
	if zone == nil {
		return "unknown"
	}
	return stringx.FirstNonEmpty(normalizeCloudflareDomain(zone.GetZoneName()), strings.TrimSpace(zone.GetZoneId()), "unknown")
}
