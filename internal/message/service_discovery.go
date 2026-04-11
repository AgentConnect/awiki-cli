package message

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/anpsdk"
)

const anpMessageServiceType = "ANPMessageService"

type discoveredAttachmentService struct {
	SenderDID   string
	ServiceDID  string
	RPCEndpoint string
}

type discoveredMessageService struct {
	ServiceDID       string
	ServiceEndpoint  string
	Profiles         []string
	SecurityProfiles []string
	Priority         int
	HasPriority      bool
}

func resolveAttachmentRPCService(
	ctx context.Context,
	senderDID string,
) (*discoveredAttachmentService, error) {
	trimmedSenderDID := strings.TrimSpace(senderDID)
	if trimmedSenderDID == "" {
		return nil, ErrAttachmentSenderRequired
	}
	document, err := anpsdk.ResolveDidDocument(ctx, trimmedSenderDID, true)
	if err != nil {
		return nil, fmt.Errorf("resolve attachment sender DID document: %w", err)
	}
	return selectAttachmentRPCServiceFromDocument(trimmedSenderDID, document)
}

func selectAttachmentRPCServiceFromDocument(
	senderDID string,
	document map[string]any,
) (*discoveredAttachmentService, error) {
	services := serviceEntriesFromDocument(document)
	candidates := make([]discoveredMessageService, 0, len(services))
	for _, service := range services {
		if service.ServiceDID == "" || service.ServiceEndpoint == "" {
			continue
		}
		if !stringSliceContains(service.Profiles, "anp.attachment.v1") {
			continue
		}
		if !stringSliceContains(service.SecurityProfiles, "transport-protected") {
			continue
		}
		candidates = append(candidates, service)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf(
			"did document %s does not expose a compatible ANPMessageService for attachments",
			senderDID,
		)
	}
	sort.SliceStable(candidates, func(i int, j int) bool {
		switch {
		case candidates[i].HasPriority && candidates[j].HasPriority:
			return candidates[i].Priority < candidates[j].Priority
		case candidates[i].HasPriority:
			return true
		case candidates[j].HasPriority:
			return false
		default:
			return false
		}
	})
	selected := candidates[0]
	if _, err := url.ParseRequestURI(selected.ServiceEndpoint); err != nil {
		return nil, fmt.Errorf("attachment service endpoint is invalid: %w", err)
	}
	return &discoveredAttachmentService{
		SenderDID:   senderDID,
		ServiceDID:  selected.ServiceDID,
		RPCEndpoint: selected.ServiceEndpoint,
	}, nil
}

func serviceEntriesFromDocument(document map[string]any) []discoveredMessageService {
	rawServices, _ := document["service"].([]any)
	services := make([]discoveredMessageService, 0, len(rawServices))
	for _, rawService := range rawServices {
		serviceMap, ok := rawService.(map[string]any)
		if !ok {
			continue
		}
		if stringFromAny(serviceMap["type"]) != anpMessageServiceType {
			continue
		}
		service := discoveredMessageService{
			ServiceDID:       stringFromAny(serviceMap["serviceDid"]),
			ServiceEndpoint:  stringFromAny(serviceMap["serviceEndpoint"]),
			Profiles:         stringSliceFromAny(serviceMap["profiles"]),
			SecurityProfiles: stringSliceFromAny(firstNonNil(serviceMap["securityProfiles"], serviceMap["security_profiles"])),
		}
		if priority, ok := priorityFromAny(serviceMap["priority"]); ok {
			service.Priority = priority
			service.HasPriority = true
		}
		services = append(services, service)
	}
	return services
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func stringSliceFromAny(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(stringFromAny(item))
		if text != "" {
			result = append(result, text)
		}
	}
	return result
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func priorityFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
