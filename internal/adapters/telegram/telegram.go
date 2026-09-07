// Package telegram implements the inbound Telegram webhook handler: it
// parses the /to_ticket command, gathers reply-chain context, extracts
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

	"github.com/actuponit/telegram-jira-integration/internal/domain"
	"github.com/actuponit/telegram-jira-integration/internal/ports"
	"github.com/actuponit/telegram-jira-integration/internal/usecase"
)

const (
	commandToTicket  = "/to_ticket"
	commandStatus    = "/status"
	secretHeaderName = "X-Telegram-Bot-Api-Secret-Token"
	usageHint        = "Usage: reply to the message you want turned into a ticket with /to_ticket [@assignee]"
	statusUsageHint  = "Usage: reply /status to one of my \"Created <KEY>: ...\" confirmation messages."

	// ticketProcessingTimeout bounds the whole asynchronous ticket flow,
	// including Telegram media download, Gemini, and Jira. It must outlive the
	// webhook response: Telegram expects a prompt acknowledgement and can
	// redeliver an update when the response is delayed.
	ticketProcessingTimeout = 2 * time.Minute
	telegramRequestTimeout  = 30 * time.Second

	// statusLookupTimeout bounds a /status lookup, which is a single Jira
	// read rather than the full drafting flow.
	statusLookupTimeout = 30 * time.Second
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
	allowlist   domain.ChatAllowlist
	botID       int64
	drafter     ports.TicketDrafter
	tracker     ports.IssueTracker
	resolver    ports.AssigneeResolver
	logger      *slog.Logger
	dispatch    func(func())
}

// New creates a Handler backed by a real Telegram bot client.
func New(botToken, secretToken string, allowlist domain.ChatAllowlist, drafter ports.TicketDrafter, tracker ports.IssueTracker, resolver ports.AssigneeResolver, logger *slog.Logger) (*Handler, error) {
	httpClient := &http.Client{Timeout: telegramRequestTimeout}
	bot, err := tgbotapi.NewBotAPIWithClient(botToken, tgbotapi.APIEndpoint, httpClient)
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot client: %w", err)
	}
	h := newHandler(bot, botFileDownloader{bot: bot, httpClient: httpClient}, secretToken, allowlist, drafter, tracker, resolver, logger)
	// Self is populated by the getMe call NewBotAPIWithClient makes; /status
	// uses it to tell the bot's own confirmation messages from a user's.
	h.botID = bot.Self.ID
	return h, nil
}

func newHandler(send sender, files fileDownloader, secretToken string, allowlist domain.ChatAllowlist, drafter ports.TicketDrafter, tracker ports.IssueTracker, resolver ports.AssigneeResolver, logger *slog.Logger) *Handler {
	return &Handler{
		send:        send,
		files:       files,
		secretToken: secretToken,
		allowlist:   allowlist,
		drafter:     drafter,
		tracker:     tracker,
		resolver:    resolver,
		logger:      logger,
		dispatch:    func(work func()) { go work() },
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
	message := update.Message

	command := commandName(message.Text)
	if command != commandToTicket && command != commandStatus {
		return
	}

	// Chat-level allowlist: a command from anywhere else gets no reply and no
	// side effects, but is still logged so an unexpected chat ID is
	// diagnosable rather than invisible.
	if !h.allowlist.Allows(message.Chat.ID) {
		h.logger.Info("command from non-allowlisted chat ignored",
			"telegram_message_id", message.MessageID,
			"chat_id", message.Chat.ID,
			"command", command,
		)
		return
	}

	// Acknowledge before making any outbound calls. The work must not inherit
	// r.Context(): it is canceled when this webhook request ends.
	switch command {
	case commandToTicket:
		h.dispatch(func() { h.processTicket(message) })
	case commandStatus:
		h.dispatch(func() { h.processStatus(message) })
	}
}

// processStatus answers a /status command sent as a reply to one of the
// bot's own "Created <KEY>: ..." confirmation messages.
func (h *Handler) processStatus(message *tgbotapi.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), statusLookupTimeout)
	defer cancel()

	logAttrs := []any{"telegram_message_id", message.MessageID, "chat_id", message.Chat.ID}

	key, ok := h.confirmationKey(message.ReplyToMessage)
	if !ok {
		h.logger.Info("status: reply target is not a bot confirmation message", append(logAttrs, "stage", "parse")...)
		h.reply(message, statusUsageHint)
		return
	}

	ticket, err := usecase.CheckTicketStatus(ctx, h.tracker, key)
	if err != nil {
		h.logger.Error("status: lookup failed", append(logAttrs, "stage", "jira", "issue_key", key, "error", err.Error())...)
		h.reply(message, fmt.Sprintf("Couldn't look up %s: %s", key, err))
		return
	}

	h.logger.Info("status: looked up issue", append(logAttrs, "stage", "success", "issue_key", ticket.Key)...)
	h.reply(message, renderStatus(ticket))
}

// confirmationKey extracts the Issue key from source only when source is
// one of this bot's own confirmation messages. A user can type text that
// looks like a confirmation, so the sender is checked too.
func (h *Handler) confirmationKey(source *tgbotapi.Message) (string, bool) {
	if source == nil || source.From == nil || source.From.ID != h.botID {
		return "", false
	}
	return ParseTicketKey(messageText(source))
}

// renderStatus is the chat rendering of a looked-up Ticket.
func renderStatus(ticket domain.Ticket) string {
	return fmt.Sprintf("%s — %s\nAssignee: %s\n%s", ticket.Key, ticket.Status, assigneeLabel(ticket.Assignee), ticket.URL)
}

// assigneeLabel renders a Ticket's assignee for chat, falling back to the
// account ID when Jira returns an assigned account with no display name.
func assigneeLabel(assignee domain.Assignee) string {
	switch {
	case assignee.IsUnassigned():
		return "Unassigned"
	case assignee.DisplayName == "":
		return assignee.AccountID
	default:
		return assignee.DisplayName
	}
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

// parseToTicketCommand reports whether text is a /to_ticket command
// (optionally addressed to this bot, e.g. "/to_ticket@MyBot"), and returns
// its optional @assignee argument, empty when omitted.
func parseToTicketCommand(text string) (assignee string, ok bool) {
	if commandName(text) != commandToTicket {
		return "", false
	}
	fields := strings.Fields(text)
	if len(fields) > 1 && strings.HasPrefix(fields[1], "@") {
		return fields[1], true
	}
	return "", true
}

// commandName returns the bare, lowercased command word of text, stripping
// any "@BotName" suffix Telegram appends in groups ("/to_ticket@MyBot foo"
// -> "/to_ticket"). It returns "" when text does not start with a command.
func commandName(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	name, _, _ := strings.Cut(fields[0], "@")
	if !strings.HasPrefix(name, "/") {
		return ""
	}
	return strings.ToLower(name)
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
