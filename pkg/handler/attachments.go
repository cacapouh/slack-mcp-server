package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/korotovsky/slack-mcp-server/pkg/server/auth"
	"github.com/korotovsky/slack-mcp-server/pkg/text"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

// Attachment represents a file attachment in Slack
type Attachment struct {
	ID         string `json:"id" csv:"id"`
	Name       string `json:"name" csv:"name"`
	Title      string `json:"title" csv:"title"`
	MimeType   string `json:"mimeType" csv:"mimeType"`
	FileType   string `json:"fileType" csv:"fileType"`
	Size       int    `json:"size" csv:"size"`
	URL        string `json:"url" csv:"url"`
	URLPrivate string `json:"urlPrivate" csv:"urlPrivate"`
	Permalink  string `json:"permalink" csv:"permalink"`
	MessageID  string `json:"messageID" csv:"messageID"`
	ChannelID  string `json:"channelID" csv:"channelID"`
	UserID     string `json:"userID" csv:"userID"`
	UserName   string `json:"userName" csv:"userName"`
	Timestamp  string `json:"timestamp" csv:"timestamp"`
	Cursor     string `json:"cursor,omitempty" csv:"cursor"`
}

// AttachmentsHandler handles attachment-related MCP tool requests
type AttachmentsHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

// NewAttachmentsHandler creates a new AttachmentsHandler
func NewAttachmentsHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *AttachmentsHandler {
	return &AttachmentsHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

// MessagesWithAttachmentsHandler searches for messages that contain file attachments
func (ah *AttachmentsHandler) MessagesWithAttachmentsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ah.logger.Debug("MessagesWithAttachmentsHandler called", zap.Any("params", request.Params))

	if authenticated, err := auth.IsAuthenticated(ctx, ah.apiProvider.ServerTransport(), ah.logger); !authenticated {
		ah.logger.Error("Authentication failed", zap.Error(err))
		return nil, err
	}

	channelID := request.GetString("channel_id", "")
	if channelID == "" {
		return nil, errors.New("channel_id is required")
	}

	limit := request.GetInt("limit", 100)
	if limit < 1 || limit > 100 {
		limit = 100
	}
	cursor := request.GetString("cursor", "")

	// Resolve channel name to ID if needed
	if strings.HasPrefix(channelID, "#") || strings.HasPrefix(channelID, "@") {
		channelsMaps := ah.apiProvider.ProvideChannelsMaps()
		chn, ok := channelsMaps.ChannelsInv[channelID]
		if !ok {
			return nil, fmt.Errorf("channel %q not found", channelID)
		}
		channelID = channelsMaps.Channels[chn].ID
	}

	// Get conversation history and filter for messages with files
	historyParams := slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Limit:     limit,
		Cursor:    cursor,
		Inclusive: false,
	}

	history, err := ah.apiProvider.Slack().GetConversationHistoryContext(ctx, &historyParams)
	if err != nil {
		ah.logger.Error("Failed to get conversation history", zap.Error(err))
		return nil, err
	}

	var attachments []Attachment
	usersMap := ah.apiProvider.ProvideUsersMap()

	for _, msg := range history.Messages {
		// Only process messages that have files
		if len(msg.Files) == 0 {
			continue
		}

		for _, file := range msg.Files {
			userName := file.User
			if user, ok := usersMap.Users[file.User]; ok {
				userName = user.Name
			}

			timestamp, err := text.TimestampToIsoRFC3339(msg.Timestamp)
			if err != nil {
				ah.logger.Warn("Failed to convert timestamp", zap.Error(err))
				timestamp = msg.Timestamp
			}

			attachment := Attachment{
				ID:         file.ID,
				Name:       file.Name,
				Title:      file.Title,
				MimeType:   file.Mimetype,
				FileType:   file.Filetype,
				Size:       file.Size,
				URL:        file.URLPrivate,
				URLPrivate: file.URLPrivateDownload,
				Permalink:  file.Permalink,
				MessageID:  msg.Timestamp,
				ChannelID:  channelID,
				UserID:     file.User,
				UserName:   userName,
				Timestamp:  timestamp,
			}
			attachments = append(attachments, attachment)
		}
	}

	// Set cursor on last item if there are more results
	if len(attachments) > 0 && history.HasMore {
		attachments[len(attachments)-1].Cursor = history.ResponseMetaData.NextCursor
	}

	csvBytes, err := gocsv.MarshalBytes(&attachments)
	if err != nil {
		ah.logger.Error("Failed to marshal attachments to CSV", zap.Error(err))
		return nil, err
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}

// GetAttachmentDetailsHandler gets detailed information about a specific file attachment
func (ah *AttachmentsHandler) GetAttachmentDetailsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ah.logger.Debug("GetAttachmentDetailsHandler called", zap.Any("params", request.Params))

	if authenticated, err := auth.IsAuthenticated(ctx, ah.apiProvider.ServerTransport(), ah.logger); !authenticated {
		ah.logger.Error("Authentication failed", zap.Error(err))
		return nil, err
	}

	fileID := request.GetString("file_id", "")
	if fileID == "" {
		return nil, errors.New("file_id is required")
	}

	// Get file info using Slack API
	file, _, _, err := ah.apiProvider.Slack().GetFileInfoContext(ctx, fileID, 1, 1)
	if err != nil {
		ah.logger.Error("Failed to get file info", zap.String("file_id", fileID), zap.Error(err))
		return nil, err
	}

	usersMap := ah.apiProvider.ProvideUsersMap()
	userName := file.User
	if user, ok := usersMap.Users[file.User]; ok {
		userName = user.Name
	}

	timestamp, err := text.TimestampToIsoRFC3339(file.Timestamp.String())
	if err != nil {
		ah.logger.Warn("Failed to convert timestamp", zap.Error(err))
		timestamp = file.Timestamp.String()
	}

	// Build detailed response with additional fields
	response := map[string]interface{}{
		"id":          file.ID,
		"name":        file.Name,
		"title":       file.Title,
		"mimeType":    file.Mimetype,
		"fileType":    file.Filetype,
		"prettyType":  file.PrettyType,
		"size":        file.Size,
		"url":         file.URLPrivate,
		"urlDownload": file.URLPrivateDownload,
		"permalink":   file.Permalink,
		"userID":      file.User,
		"userName":    userName,
		"timestamp":   timestamp,
		"isPublic":    file.IsPublic,
		"isExternal":  file.IsExternal,
		"channels":    file.Channels,
		"groups":      file.Groups,
		"ims":         file.IMs,
	}

	// Add thumbnail URLs if available
	thumbnails := map[string]string{}
	if file.Thumb64 != "" {
		thumbnails["thumb64"] = file.Thumb64
	}
	if file.Thumb80 != "" {
		thumbnails["thumb80"] = file.Thumb80
	}
	if file.Thumb160 != "" {
		thumbnails["thumb160"] = file.Thumb160
	}
	if file.Thumb360 != "" {
		thumbnails["thumb360"] = file.Thumb360
	}
	if file.Thumb480 != "" {
		thumbnails["thumb480"] = file.Thumb480
	}
	if file.Thumb720 != "" {
		thumbnails["thumb720"] = file.Thumb720
	}
	if file.Thumb1024 != "" {
		thumbnails["thumb1024"] = file.Thumb1024
	}
	if len(thumbnails) > 0 {
		response["thumbnails"] = thumbnails
	}

	// Add image dimensions if available
	if file.OriginalW > 0 || file.OriginalH > 0 {
		response["dimensions"] = map[string]int{
			"width":  file.OriginalW,
			"height": file.OriginalH,
		}
	}

	jsonBytes, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		ah.logger.Error("Failed to marshal response to JSON", zap.Error(err))
		return nil, err
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}
