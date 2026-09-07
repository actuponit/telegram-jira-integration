// Package telegram implements the inbound Telegram webhook handler: it
// parses the /to-ticket command, gathers reply-chain context, extracts
// media, calls usecase.CreateTicketFromMessage, and renders the result
// back into the chat.
package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/actuponit/telegram-jira-integration/internal/ports"
	"github.com/actuponit/telegram-jira-integration/internal/usecase"
)

const (
	command          = "/to-ticket"
	secretHeaderName = "X-Telegram-Bot-Api-Secret-Token"
	usageHint        = "Usage: reply to the message you want turned into a ticket with /to-ticket [@assignee]"

	// ticketProcessingTimeout bounds the whole asynchronous ticket flow,
	// including Telegram media download, Gemini, and Jira. It must outlive the
	// webhook response: Telegram expects a prompt acknowledgement and can
	// redeliver an update when the response is delayed.
	ticketProcessingTimeout = 2 * time.Minute
	telegramRequestTimeout  = 30 * time.Second
)

// sender is the subset of *tgbotapi.BotAPI this adapter uses to reply into
// a chat. Abstracted so tests can fake it instead of calling Telegram.
type sender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// fileDownloader is the subset of Telegram file access this adapter needs:
// resolving a file ID to a download URL, and fetching the bytes at a URL.
// Abstracted so tests can fake it instead of calling Telegram.
type fileDownloader interface {
	FileURL(fileID string) (string, error)
	Download(ctx context.Context, url string) ([]byte, error)
}

// Handler is the inbound Telegram webhook adapter.
type Handler struct {
	send        sender
	files       fileDownloader
	secretToken string
	drafter     ports.TicketDrafter
	tracker     ports.IssueTracker
	resolver    ports.AssigneeResolver
	logger      *slog.Logger
}

// New creates a Handler backed by a real Telegram bot client.
func New(botToken, secretToken string, drafter ports.TicketDrafter, tracker ports.IssueTracker, resolver ports.AssigneeResolver, logger *slog.Logger) (*Handler, error) {
	httpClient := &http.Client{Timeout: telegramRequestTimeout}
	bot, err := tgbotapi.NewBotAPIWithClient(botToken, tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot client: %w", err)
	}
	return newHandler(bot, botFileDownloader{bot: bot, httpClient: httpClient}, secretToken, drafter, tracker, resolver, logger), nil
}

func newHandler(send sender, files fileDownloader, secretToken string, drafter ports.TicketDrafter, tracker ports.IssueTracker, resolver ports.AssigneeResolver, logger *slog.Logger) *Handler {
	return &Handler{
		send:        send,
		files:       files,
		secretToken: secretToken,
		drafter:     drafter,
		tracker:     tracker,
		resolver:    resolver,
		logger:      logger,
	}
}

// botFileDownloader implements fileDownloader against a real *tgbotapi.BotAPI.
type botFileDownloader struct {
	bot        *tgbotapi.BotAPI
	httpClient *http.Client
}

func (d botFileDownloader) FileURL(fileID string) (string, error) {
	return d.bot.GetFileDirectURL(fileID)
}

func (d botFileDownloader) Download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("telegram: build download request: %w", err)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: download file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram: download file: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func secretTokenMatches(got, want string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// ServeHTTP handles one Telegram webhook delivery.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.secretToken == "" || !secretTokenMatches(r.Header.Get(secretHeaderName), h.secretToken) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var update tgbotapi.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)

	if update.Message == nil {
		return
	}
	if _, isTicketCommand := parseToTicketCommand(update.Message.Text); !isTicketCommand {
		return
	}

	// Acknowledge before making any outbound calls. The work must not inherit
	// r.Context(): it is canceled when this webhook request ends.
	go h.processTicket(update.Message)
}

func (h *Handler) processTicket(message *tgbotapi.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), ticketProcessingTimeout)
	defer cancel()

	assigneeHandle, _ := parseToTicketCommand(message.Text)
	logAttrs := []any{"telegram_message_id", message.MessageID, "chat_id", message.Chat.ID}

	if message.ReplyToMessage == nil {
		h.logger.Info("to-ticket: no reply target", logAttrs...)
		h.reply(message, usageHint)
		return
	}

	source := message.ReplyToMessage
	contextMessages := gatherContext(source)

	attachment, err := extractImage(ctx, source, h.files)
	if err != nil {
		h.logger.Error("to-ticket: image download failed", append(logAttrs, "stage", "image_download", "error", err.Error())...)
		h.reply(message, "Couldn't download the image on that message — try again or file the ticket manually.")
		return
	}

	videoURL, err := extractVideoURL(ctx, source, h.files)
	if err != nil {
		h.logger.Error("to-ticket: video URL resolution failed", append(logAttrs, "stage", "video_lookup", "error", err.Error())...)
		h.reply(message, "Couldn't resolve the video on that message — try again or file the ticket manually.")
		return
	}

	req := usecase.CreateTicketRequest{
		ContextMessages: contextMessages,
		TelegramHandle:  assigneeHandle,
		ImageAttachment: attachment,
		VideoURL:        videoURL,
	}

	result, err := usecase.CreateTicketFromMessage(ctx, h.drafter, h.tracker, h.resolver, req)
	if err != nil {
		stage, reply := classifyCreateTicketError(err)
		h.logger.Error("to-ticket: create ticket failed", append(logAttrs, "stage", stage, "error", err.Error())...)
		h.reply(message, reply)
		return
	}

	reply := fmt.Sprintf("Created %s: %s — %s", result.Ticket.Key, result.Title, result.Ticket.URL)
	if result.AssigneeUnresolved {
		reply += fmt.Sprintf("\n\nCouldn't resolve assignee %q — created unassigned.", assigneeHandle)
	}
	h.logger.Info("to-ticket: created issue", append(logAttrs, "stage", "success", "issue_key", result.Ticket.Key)...)
	h.reply(message, reply)
}

func (h *Handler) reply(to *tgbotapi.Message, text string) {
	msg := tgbotapi.NewMessage(to.Chat.ID, text)
	msg.ReplyToMessageID = to.MessageID
	if _, err := h.send.Send(msg); err != nil {
		h.logger.Error("to-ticket: send reply failed", "chat_id", to.Chat.ID, "error", err.Error())
	}
}

// classifyCreateTicketError distinguishes a drafting (Gemini) failure from
// an issue-creation (Jira) failure via the sentinel errors
// usecase.CreateTicketFromMessage wraps its errors with, and renders a
// chat-facing message for each.
func classifyCreateTicketError(err error) (stage, chatMessage string) {
	switch {
	case errors.Is(err, usecase.ErrDraftFailed):
		return "gemini", fmt.Sprintf("Couldn't draft the ticket (Gemini failed or timed out): %s", err)
	case errors.Is(err, usecase.ErrCreateIssueFailed):
		return "jira", fmt.Sprintf("Couldn't create the Jira issue: %s", err)
	default:
		return "unknown", fmt.Sprintf("Couldn't create the ticket: %s", err)
	}
}

// parseToTicketCommand reports whether text is a /to-ticket command
// (optionally addressed to this bot, e.g. "/to-ticket@MyBot"), and returns
// its optional @assignee argument, empty when omitted.
func parseToTicketCommand(text string) (assignee string, ok bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", false
	}
	name, _, _ := strings.Cut(fields[0], "@")
	if !strings.EqualFold(name, command) {
		return "", false
	}
	if len(fields) > 1 && strings.HasPrefix(fields[1], "@") {
		return fields[1], true
	}
	return "", true
}

// gatherContext walks source's own reply chain and returns it as
// []ports.Message ordered oldest to newest, with source last. Telegram
// itself only populates one level of ReplyToMessage per webhook update, so
// in practice this rarely exceeds two messages — the loop still walks as
// deep as whatever the payload provides.
func gatherContext(source *tgbotapi.Message) []ports.Message {
	var chain []*tgbotapi.Message
	for m := source; m != nil; m = m.ReplyToMessage {
		chain = append(chain, m)
	}

	messages := make([]ports.Message, len(chain))
	for i, m := range chain {
		messages[len(chain)-1-i] = ports.Message{
			SenderName: senderName(m),
			Text:       messageText(m),
			Timestamp:  int64(m.Date),
		}
	}
	return messages
}

func senderName(m *tgbotapi.Message) string {
	if m.From == nil {
		return "unknown"
	}
	if m.From.UserName != "" {
		return m.From.UserName
	}
	return m.From.FirstName
}

func messageText(m *tgbotapi.Message) string {
	if m.Text != "" {
		return m.Text
	}
	return m.Caption
}

// extractImage downloads the largest photo on source, if any. Returns nil,
// nil when source carries no photo.
func extractImage(ctx context.Context, source *tgbotapi.Message, files fileDownloader) (*ports.Attachment, error) {
	if len(source.Photo) == 0 {
		return nil, nil
	}
	largest := source.Photo[len(source.Photo)-1]

	url, err := files.FileURL(largest.FileID)
	if err != nil {
		return nil, fmt.Errorf("resolve image file URL: %w", err)
	}
	data, err := files.Download(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("download image: %w", err)
	}

	return &ports.Attachment{
		Filename: filenameFromURL(url, "image.jpg"),
		Data:     data,
	}, nil
}

// extractVideoURL resolves the download URL of the video on source, if
// any, without downloading it. Returns "", nil when source carries no video.
func extractVideoURL(_ context.Context, source *tgbotapi.Message, files fileDownloader) (string, error) {
	if source.Video == nil {
		return "", nil
	}
	url, err := files.FileURL(source.Video.FileID)
	if err != nil {
		return "", fmt.Errorf("resolve video file URL: %w", err)
	}
	return url, nil
}

func filenameFromURL(url, fallback string) string {
	base := path.Base(url)
	if base == "." || base == "/" || base == "" {
		return fallback
	}
	return base
}

var confirmationKeyPattern = regexp.MustCompile(`^Created ([A-Z][A-Z0-9]*-\d+):`)

// ParseTicketKey extracts the Issue key from the bot's own confirmation
// message ("Created <KEY>: <title> — <url>"). It is a pure string-parsing
// function with no port dependency, used by ticket 04's /status command.
func ParseTicketKey(text string) (string, bool) {
	match := confirmationKeyPattern.FindStringSubmatch(text)
	if match == nil {
		return "", false
	}
	return match[1], true
}
