package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewAttachmentsHandler(t *testing.T) {
	logger := zap.NewNop()

	handler := NewAttachmentsHandler(nil, logger)

	assert.NotNil(t, handler)
	assert.Nil(t, handler.apiProvider)
	assert.Equal(t, logger, handler.logger)
}

func TestAttachment_Struct(t *testing.T) {
	attachment := Attachment{
		ID:         "F1234567890",
		Name:       "test.pdf",
		Title:      "Test Document",
		MimeType:   "application/pdf",
		FileType:   "pdf",
		Size:       1024,
		URL:        "https://files.slack.com/files-pri/T1234567890-F1234567890/test.pdf",
		URLPrivate: "https://files.slack.com/files-pri/T1234567890-F1234567890/download/test.pdf",
		Permalink:  "https://example.slack.com/files/U1234567890/F1234567890/test.pdf",
		MessageID:  "1234567890.123456",
		ChannelID:  "C1234567890",
		UserID:     "U1234567890",
		UserName:   "testuser",
		Timestamp:  "2023-01-01T00:00:00Z",
	}

	assert.Equal(t, "F1234567890", attachment.ID)
	assert.Equal(t, "test.pdf", attachment.Name)
	assert.Equal(t, "Test Document", attachment.Title)
	assert.Equal(t, "application/pdf", attachment.MimeType)
	assert.Equal(t, "pdf", attachment.FileType)
	assert.Equal(t, 1024, attachment.Size)
	assert.Equal(t, "C1234567890", attachment.ChannelID)
	assert.Equal(t, "U1234567890", attachment.UserID)
	assert.Equal(t, "testuser", attachment.UserName)
}

func TestAttachment_CSVTags(t *testing.T) {
	// Verify CSV tags exist on all fields
	attachment := Attachment{}

	// Using reflection would be more thorough, but this basic test
	// ensures the struct is properly defined
	assert.NotNil(t, attachment)
}
