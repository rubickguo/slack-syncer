package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/jung-kurt/gofpdf"
	"github.com/rusq/slack"
	"github.com/rusq/slackdump/v3"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	_ "modernc.org/sqlite"
)

// App struct
type App struct {
	ctx          context.Context
	session      *slackdump.Session
	db           *sql.DB
	users        map[string]slack.User
	channelNames map[string]string
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		users:        make(map[string]slack.User),
		channelNames: make(map[string]string),
	}
}

// ... (startup and initDB remain same) ...

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.initDB("slackdump.db"); err != nil {
		log.Println("Failed to init DB:", err)
	}
}

func (a *App) initDB(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	query := `
	CREATE TABLE IF NOT EXISTS messages (
		ts TEXT,
		channel_id TEXT,
		user_id TEXT,
		text TEXT,
		thread_ts TEXT,
		json TEXT,
		PRIMARY KEY (ts, channel_id)
	);
	`
	if _, err := db.Exec(query); err != nil {
		return err
	}
	a.db = db
	return nil
}

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Topic       string `json:"topic"`
	MemberCount int    `json:"member_count"`
}

func (a *App) SelectOutputDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Export Output Directory",
	})
}

func (a *App) Login(workspace string, dCookie string) (string, error) {
	workspaceName, err := normalizeWorkspaceName(workspace)
	if err != nil {
		return "", err
	}

	token, cookies, err := getTokenByCookie(a.ctx, workspaceName, dCookie)
	if err != nil {
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	prov := &SimpleProvider{Token: token, Cookie: cookies}
	sess, err := slackdump.New(a.ctx, prov)
	if err != nil {
		return "", fmt.Errorf("failed to initialize session: %w", err)
	}

	a.session = sess

	// Prefetch users in background
	go a.fetchAllUsers()
	// Prefetch channels to build name cache
	go a.GetChannels()

	return sess.Info().User, nil
}

func (a *App) fetchAllUsers() {
	if a.session == nil {
		return
	}
	// This might take a while for large workspaces
	users, err := a.session.GetUsers(context.Background())
	if err == nil {
		for _, u := range users {
			a.users[u.ID] = u
		}
		fmt.Printf("Cached %d users\n", len(users))
	}
}

// ... (LoginWithBrowser) ...

// LoginWithBrowser opens a browser for the user to login and captures the 'd' cookie
func (a *App) LoginWithBrowser(workspaceURL string) (string, error) {
	normalizedURL, err := normalizeWorkspaceURL(workspaceURL)
	if err != nil {
		return "", err
	}

	// Launch browser
	browser := rod.New().MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(normalizedURL)

	// Default Wait for 5 minutes max
	maxWait := time.Now().Add(5 * time.Minute)

	fmt.Println("Browser opened. Waiting for login...")

	for time.Now().Before(maxWait) {
		cookies, err := page.Cookies([]string{normalizedURL})
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		for _, c := range cookies {
			if c.Name == "d" {
				// Found it!
				return c.Value, nil
			}
		}

		time.Sleep(1 * time.Second)
	}

	return "", fmt.Errorf("timeout waiting for login")
}

func (a *App) GetChannels() ([]Channel, error) {
	if a.session == nil {
		return nil, errors.New("not authenticated")
	}
	chs, err := a.session.GetChannels(a.ctx)
	if err != nil {
		return nil, err
	}
	var result []Channel
	for _, c := range chs {
		a.channelNames[c.ID] = c.Name // Cache Name
		result = append(result, Channel{
			ID:          c.ID,
			Name:        c.Name,
			Topic:       c.Topic.Value,
			MemberCount: c.NumMembers,
		})
	}
	return result, nil
}

// ... (StartSync) ...

func (a *App) StartSync(channelIDs []string, baseDir string, formats []string) error {
	if a.session == nil {
		return errors.New("not authenticated")
	}
	if baseDir == "" {
		baseDir = "."
	}

	// Ensure DB layout
	if err := a.setupDB(baseDir, formats); err != nil {
		return err
	}

	// Ensure channel cache is populated
	if len(a.channelNames) == 0 {
		a.GetChannels()
	}

	for _, cid := range channelIDs {
		fmt.Printf("Syncing channel %s...\n", cid)
		err := a.syncChannel(cid, baseDir, formats)
		if err != nil {
			fmt.Printf("Error syncing %s: %v\n", cid, err)
		}
	}
	return nil
}

func (a *App) StartSyncURLs(urls []string, baseDir string, formats []string) error {
	if a.session == nil {
		return errors.New("not authenticated")
	}
	if baseDir == "" {
		baseDir = "."
	}

	if err := a.setupDB(baseDir, formats); err != nil {
		return err
	}

	// Ensure channel cache is populated
	if len(a.channelNames) == 0 {
		a.GetChannels()
	}

	visited := make(map[string]bool)

	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}

		cid, ts, err := parseSlackURL(u)
		if err != nil {
			fmt.Printf("Invalid URL %s: %v\n", u, err)
			continue
		}

		key := cid + "_" + ts
		if visited[key] {
			continue
		}
		visited[key] = true

		fmt.Printf("Syncing Thread %s / %s ...\n", cid, ts)

		replies, err := a.fetchReplies(cid, ts)
		if err != nil {
			fmt.Printf("Failed to fetch thread %s: %v\n", u, err)
			continue
		}
		if len(replies) == 0 {
			fmt.Printf("No messages found for %s\n", u)
			continue
		}

		// Save to DB
		if a.db != nil {
			a.saveMessagesToDB(cid, replies)
		}

		// Save Files
		if err := a.saveThreadFiles(cid, replies, baseDir, formats); err != nil {
			fmt.Printf("Failed to save files for %s: %v\n", u, err)
		}
	}
	return nil
}

func parseSlackURL(s string) (string, string, error) {
	// 1. Parse URL object to get query params
	uObj, err := url.Parse(s)
	if err == nil {
		// Check for thread_ts parameter (this is the root if present)
		// e.g. ?thread_ts=1679654279.815259&cid=C12345
		q := uObj.Query()
		if val := q.Get("thread_ts"); val != "" {
			// We still need Channel ID.
			// Format: /archives/C12345/p12345...
			// The CID is usually in the path.
			// Let's rely on path parsing for CID, but use thread_ts for TS if present.
			// Continue path parsing...
		}
	}

	// Standard Path: /archives/C12345/p12345...
	parts := strings.Split(s, "/archives/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("no archives segment")
	}

	seg := parts[1] // C12345678/p1679654279815259...

	// Handle case where query params might be attached to seg if raw string split
	if idx := strings.Index(seg, "?"); idx != -1 {
		seg = seg[:idx]
	}

	subParts := strings.Split(seg, "/")
	if len(subParts) < 2 {
		return "", "", fmt.Errorf("invalid format")
	}

	cid := subParts[0]
	rawTS := subParts[1] // p1679654279815259 or similar

	// Refined Logic: Check if there was a thread_ts in the original URL query
	if uObj != nil {
		q := uObj.Query()
		if tts := q.Get("thread_ts"); tts != "" {
			// Found explicit thread root!
			return cid, tts, nil
		}
	}

	// Fallback to path timestamp
	if !strings.HasPrefix(rawTS, "p") {
		return "", "", fmt.Errorf("timestamp must start with p")
	}

	// Convert p1679654279815259 -> 1679654279.815259
	nums := rawTS[1:]
	if len(nums) < 7 {
		return "", "", fmt.Errorf("timestamp too short")
	}

	ts := nums[:len(nums)-6] + "." + nums[len(nums)-6:]
	return cid, ts, nil
}

func (a *App) setupDB(baseDir string, formats []string) error {
	for _, f := range formats {
		if f == "sqlite" {
			dbPath := filepath.Join(baseDir, "slackdump.db")
			if a.db != nil {
				a.db.Close()
			}
			if err := a.initDB(dbPath); err != nil {
				return fmt.Errorf("failed to switch DB: %w", err)
			}
			break
		}
	}
	return nil
}

func (a *App) syncChannel(cid string, baseDir string, formats []string) error {
	client := a.session.Client()

	// Create Channel Directory
	channelDir := filepath.Join(baseDir, cid)
	if err := os.MkdirAll(channelDir, 0755); err != nil {
		return err
	}

	// JSON Aggregation
	var jsonFile *os.File
	firstJSON := true
	for _, f := range formats {
		if f == "json" {
			f, _ := os.Create(filepath.Join(channelDir, "_all_messages.json"))
			jsonFile = f
			defer jsonFile.Close()
			fmt.Fprintln(jsonFile, "[")
		}
	}

	var cursor string

	for {
		params := &slack.GetConversationHistoryParameters{ChannelID: cid, Cursor: cursor, Limit: 50}
		hist, err := client.GetConversationHistoryContext(a.ctx, params)
		if err != nil {
			if strings.Contains(err.Error(), "paid_history") {
				return nil
			}
			if strings.Contains(err.Error(), "ratelimited") {
				time.Sleep(2 * time.Second)
				continue
			}
			return err
		}

		for _, m := range hist.Messages {
			if m.SubType == "channel_join" || m.SubType == "channel_leave" || m.SubType == "group_join" || m.SubType == "group_leave" {
				continue
			}

			// Fetch Thread
			msgsToProcess := []slack.Message{m}
			if m.ReplyCount > 0 && m.ThreadTimestamp != "" {
				replies, err := a.fetchReplies(cid, m.ThreadTimestamp)
				if err == nil {
					if len(replies) > 0 && replies[0].Timestamp == m.Timestamp {
						msgsToProcess = append(msgsToProcess, replies[1:]...)
					} else {
						msgsToProcess = append(msgsToProcess, replies...)
					}
				}
			}

			// DB & JSON (Aggregated)
			for _, msg := range msgsToProcess {
				if a.db != nil {
					a.saveMessagesToDB(cid, []slack.Message{msg})
				}
				if jsonFile != nil {
					if !firstJSON {
						fmt.Fprintln(jsonFile, ",")
					}
					firstJSON = false
					bytes, _ := json.MarshalIndent(msg, "  ", "  ")
					fmt.Fprint(jsonFile, string(bytes))
				}
			}

			// Files
			a.saveThreadFiles(cid, msgsToProcess, baseDir, formats)
		}

		if !hist.HasMore || hist.ResponseMetaData.NextCursor == "" {
			break
		}
		cursor = hist.ResponseMetaData.NextCursor
		time.Sleep(200 * time.Millisecond)
	}

	if jsonFile != nil {
		fmt.Fprintln(jsonFile, "\n]")
	}
	return nil
}

func (a *App) saveThreadFiles(cid string, msgs []slack.Message, baseDir string, formats []string) error {
	if len(msgs) == 0 {
		return nil
	}

	// Resolve Folder Name: use Name if available, else ID
	folderName := cid
	if name, ok := a.channelNames[cid]; ok && name != "" {
		folderName = name
	}

	// Ensure dir exists
	channelDir := filepath.Join(baseDir, folderName)
	if err := os.MkdirAll(channelDir, 0755); err != nil {
		return err
	}

	rootMsg := msgs[0]
	filename := sanitizeFilename(rootMsg.Text)
	if filename == "" {
		filename = "msg_" + rootMsg.Timestamp
	}

	rootTS := time.Unix(parseTimestamp(rootMsg.Timestamp), 0).Format("2006-01-02_150405")
	baseName := fmt.Sprintf("%s_%s", rootTS, filename)

	// Avoid filename too long issues for path
	if len(baseName) > 100 {
		baseName = baseName[:100]
	}

	for _, f := range formats {
		if f == "markdown" {
			path := filepath.Join(channelDir, baseName+".md")
			writeMarkdownThread(path, cid, msgs, a.users)
		}
		if f == "pdf" {
			path := filepath.Join(channelDir, baseName+".pdf")
			writePDFThread(path, cid, msgs, a.users)
		}
		if f == "html" {
			// For HTML, nice to have a darker unique name to avoid collision if truncated
			path := filepath.Join(channelDir, baseName+".html")
			writeHTMLThread(path, cid, msgs, a.users)
		}
		if f == "json" {
			// For URL sync, we might want individual JSONs too?
			// Let's add supported "json_thread" logic implicitly or just skip if handled by aggregator
			// If not aggregating (e.g. URL sync), dump individual JSON?
			// The caller 'syncChannel' handles aggregation. 'StartSyncURLs' calls this.
			// Let's output individual JSON if requested, overwriting logic maybe?
			// Actually, let's just make URL sync output per-thread JSON.
			// Currently 'f=="json"' is checked here.
			// In syncChannel, it does aggregator.
			// Let's keep this function purely for file-per-thread formats.
			// I'll add JSON export to StartSyncURLs loop explicitly if needed.
		}
	}
	return nil
}

// sanitizeFilename cleans string for use as filename
func sanitizeFilename(text string) string {
	// First line only
	if idx := strings.Index(text, "\n"); idx != -1 {
		text = text[:idx]
	}
	// Max chars
	runes := []rune(text)
	if len(runes) > 30 {
		text = string(runes[:30])
	}
	// Regex for invalid chars (Windows/Unix)
	reg := regexp.MustCompile(`[<>:"/\\|?*]`)
	text = reg.ReplaceAllString(text, "_")
	return strings.TrimSpace(text)
}

func writeMarkdownThread(path, cid string, msgs []slack.Message, users map[string]slack.User) {
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()

	// Title
	if len(msgs) > 0 {
		fmt.Fprintf(file, "# %s\n\n", msgs[0].Text)
	}

	for _, msg := range msgs {
		name := resolveName(msg.User, users)
		ts := time.Unix(parseTimestamp(msg.Timestamp), 0).Format("2006-01-02 15:04:05")

		indent := ""
		if msg.ThreadTimestamp != "" && msg.ThreadTimestamp != msg.Timestamp {
			indent = "> "
		}

		fmt.Fprintf(file, "%s**%s** [%s]:\n%s%s\n\n", indent, name, ts, indent, strings.ReplaceAll(msg.Text, "\n", "\n"+indent))
	}
}

func writePDFThread(path, cid string, msgs []slack.Message, users map[string]slack.User) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "", 10)

	// Add Title (Root msg)
	if len(msgs) > 0 {
		pdf.SetFont("Arial", "B", 14)
		// Basic sanitization, PDF requires proper font for non-latin, this is 'best effort'
		pdf.MultiCell(0, 10, sanitizePDFText(msgs[0].Text), "", "", false)
		pdf.Ln(5)
		pdf.SetFont("Arial", "", 10)
	}

	tr := pdf.UnicodeTranslatorFromDescriptor("")

	for _, msg := range msgs {
		name := resolveName(msg.User, users)
		ts := time.Unix(parseTimestamp(msg.Timestamp), 0).Format("2006-01-02 15:04:05")

		if msg.ThreadTimestamp != "" && msg.ThreadTimestamp != msg.Timestamp {
			pdf.SetLeftMargin(20)
		}

		pdf.SetFont("Arial", "B", 10)
		pdf.Write(5, fmt.Sprintf("%s [%s]\n", name, ts))
		pdf.SetFont("Arial", "", 10)

		cleanText := sanitizePDFText(msg.Text)
		pdf.MultiCell(0, 5, tr(cleanText), "", "", false)
		pdf.Ln(2)

		if msg.ThreadTimestamp != "" && msg.ThreadTimestamp != msg.Timestamp {
			pdf.SetLeftMargin(10) // Reset
		}
	}
	pdf.OutputFileAndClose(path)
}

func resolveName(uid string, users map[string]slack.User) string {
	if u, ok := users[uid]; ok {
		if u.RealName != "" {
			return u.RealName
		}
		return u.Name
	}
	return uid
}

func sanitizePDFText(s string) string {
	s = strings.ReplaceAll(s, "\t", "    ")
	// Remove non-printable if needed, simple pass
	return s
}

func (a *App) fetchReplies(cid, ts string) ([]slack.Message, error) {
	var msgs []slack.Message
	var cursor string
	for {
		params := &slack.GetConversationRepliesParameters{
			ChannelID: cid,
			Timestamp: ts,
			Cursor:    cursor,
			Limit:     200,
		}
		messages, hasMore, next, err := a.session.Client().GetConversationRepliesContext(a.ctx, params)
		if err != nil {
			if strings.Contains(err.Error(), "ratelimited") {
				time.Sleep(1 * time.Second)
				continue
			}
			return msgs, err
		}
		msgs = append(msgs, messages...)
		if !hasMore || next == "" {
			break
		}
		cursor = next
		time.Sleep(100 * time.Millisecond)
	}
	return msgs, nil
}

// ... (Helper functions: parseTimestamp, saveMessagesToDB, Login helpers remain similar) ...

func parseTimestamp(ts string) int64 {
	parts := strings.Split(ts, ".")
	if len(parts) > 0 {
		var sec int64
		fmt.Sscanf(parts[0], "%d", &sec)
		return sec
	}
	return 0
}

func (a *App) saveMessagesToDB(channelID string, msgs []slack.Message) error {
	if a.db == nil {
		return nil
	}
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT OR REPLACE INTO messages (ts, channel_id, user_id, text, thread_ts, json) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, m := range msgs {
		raw, _ := json.Marshal(m)
		_, err := stmt.Exec(m.Timestamp, channelID, m.User, m.Text, m.ThreadTimestamp, string(raw))
		if err != nil {
			log.Println("DB Error:", err)
		}
	}
	return tx.Commit()
}

// ... Auth Logic ...
var tokenRegex = regexp.MustCompile(`"api_token":"([^"]+)"`)
var ssbURI = func(workspace string) string { return "https://" + workspace + ".slack.com/ssb/redirect" }

func normalizeWorkspaceName(input string) (string, error) {
	workspace := strings.TrimSpace(input)
	if workspace == "" {
		return "", errors.New("workspace is empty")
	}

	if strings.HasPrefix(workspace, "http://") || strings.HasPrefix(workspace, "https://") {
		u, err := url.Parse(workspace)
		if err != nil {
			return "", fmt.Errorf("invalid workspace URL: %w", err)
		}
		workspace = u.Hostname()
	}

	workspace = strings.TrimPrefix(workspace, "https://")
	workspace = strings.TrimPrefix(workspace, "http://")
	workspace = strings.TrimSuffix(workspace, "/")

	if idx := strings.IndexRune(workspace, '/'); idx >= 0 {
		workspace = workspace[:idx]
	}

	workspace = strings.TrimSuffix(workspace, ".slack.com")
	workspace = strings.TrimSpace(workspace)

	if workspace == "" {
		return "", errors.New("workspace is empty")
	}
	if strings.ContainsAny(workspace, " \t") {
		return "", errors.New("workspace contains spaces")
	}

	return workspace, nil
}

func normalizeWorkspaceURL(input string) (string, error) {
	workspace, err := normalizeWorkspaceName(input)
	if err != nil {
		return "", err
	}
	return "https://" + workspace + ".slack.com", nil
}

func getTokenByCookie(ctx context.Context, workspaceName string, dCookie string) (string, []*http.Cookie, error) {
	if dCookie == "" {
		return "", nil, errors.New("cookie is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ssbURI(workspaceName), nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Add("Cookie", "d="+dCookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	token, err := extractToken(resp.Body)
	if err != nil {
		return "", nil, err
	}
	cookies := resp.Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "d" {
			found = true
			break
		}
	}
	if !found {
		cookies = append(cookies, &http.Cookie{Name: "d", Value: dCookie})
	}
	return token, cookies, nil
}

func extractToken(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", errors.New("token missing")
			} else {
				return "", err
			}
		}
		if !strings.Contains(line, "api_token") {
			continue
		}
		matches := tokenRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			return matches[1], nil
		}
	}
}

type SimpleProvider struct {
	Token  string
	Cookie []*http.Cookie
}

func (s *SimpleProvider) SlackToken() string      { return s.Token }
func (s *SimpleProvider) Cookies() []*http.Cookie { return s.Cookie }
func (s *SimpleProvider) Validate() error         { return nil }
func (s *SimpleProvider) Test(ctx context.Context) (*slack.AuthTestResponse, error) {
	cl := slack.New(s.Token, slack.OptionHTTPClient(s.HTTPClientIgnoringErr()))
	return cl.AuthTestContext(ctx)
}
func (s *SimpleProvider) HTTPClient() (*http.Client, error) {
	return &http.Client{Jar: &cookieJar{cookies: s.Cookie}}, nil
}
func (s *SimpleProvider) HTTPClientIgnoringErr() *http.Client { c, _ := s.HTTPClient(); return c }

type cookieJar struct{ cookies []*http.Cookie }

func (j *cookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {}
func (j *cookieJar) Cookies(u *url.URL) []*http.Cookie             { return j.cookies }

func writeHTMLThread(path, cid string, msgs []slack.Message, users map[string]slack.User) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	title := "Chat Log"
	if len(msgs) > 0 {
		title = msgs[0].Text
	}
	if len(title) > 50 {
		title = title[:50] + "..."
	}

	// Simple Modern Chat Template
	const tplHead = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; background: #f4f6f8; margin: 0; padding: 20px; color: #1d1c1d; }
.container { max-width: 800px; margin: 0 auto; background: #fff; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); padding: 20px; }
.msg { display: flex; margin-bottom: 20px; align-items: flex-start; }
.msg.reply { margin-left: 50px; border-left: 2px solid #ddd; padding-left: 10px; }
.avatar { width: 40px; height: 40px; border-radius: 4px; background: #e0e0e0; display: flex; align-items: center; justify-content: center; font-weight: bold; color: #555; margin-right: 12px; flex-shrink: 0; }
.content { flex: 1; min-width: 0; }
.header { margin-bottom: 4px; font-weight: bold; font-size: 15px; }
.time { font-weight: normal; font-size: 12px; color: #616061; margin-left: 8px; }
.text { font-size: 15px; line-height: 1.5; white-space: pre-wrap; word-break: break-word; }
code { background: #f8f8f8; padding: 2px 4px; border-radius: 3px; font-family: Monaco, monospace; font-size: 0.9em; border: 1px solid #eee; }
pre { background: #f8f8f8; padding: 10px; border-radius: 4px; overflow-x: auto; border: 1px solid #eee; }
blockquote { border-left: 4px solid #ddd; margin: 0; padding-left: 10px; color: #555; }
</style>
</head>
<body>
<div class="container">
`
	fmt.Fprintf(f, tplHead, htmlEscape(title))

	for _, m := range msgs {
		user := users[m.User]
		name := user.RealName
		if name == "" {
			name = user.Name
		}
		if name == "" {
			name = m.User
		}

		ts := time.Unix(parseTimestamp(m.Timestamp), 0).Format("2006-01-02 15:04:05")

		initials := ""
		if len(name) > 0 {
			initials = string([]rune(name)[0])
		}
		if len(name) > 1 && strings.Contains(name, " ") {
			parts := strings.Fields(name)
			if len(parts) > 1 {
				initials = string([]rune(parts[0])[0]) + string([]rune(parts[1])[0])
			}
		}
		initials = strings.ToUpper(initials)

		// Color gen (simple hash)
		hash := 0
		for _, c := range name {
			hash = int(c) + ((hash << 5) - hash)
		}
		hue := hash % 360
		if hue < 0 {
			hue += 360
		}
		avatarColor := fmt.Sprintf("hsl(%d, 60%%, 85%%)", hue)

		isReply := m.ThreadTimestamp != "" && m.ThreadTimestamp != m.Timestamp
		wrapperClass := "msg"
		if isReply {
			wrapperClass += " reply"
		}

		fmt.Fprintf(f, `<div class="%s">
	<div class="avatar" style="background: %s">%s</div>
	<div class="content">
		<div class="header">%s <span class="time">%s</span></div>
		<div class="text">%s</div>
	</div>
</div>`,
			wrapperClass, avatarColor, htmlEscape(initials),
			htmlEscape(name), ts,
			customFormatText(m.Text)) // Basic formatting
	}

	fmt.Fprintln(f, "</div>\n</body>\n</html>")
}

func htmlEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "&", "&amp;"), "<", "&lt;")
}

func customFormatText(s string) string {
	// Simple linkifier and formatting helper
	s = htmlEscape(s)
	// Bold
	s = regexp.MustCompile(`\*(.*?)\*`).ReplaceAllString(s, "<b>$1</b>")
	// Italic
	s = regexp.MustCompile(`_(.*?)_`).ReplaceAllString(s, "<i>$1</i>")
	// Pre
	s = regexp.MustCompile("```([^`]+)```").ReplaceAllString(s, "<pre>$1</pre>")
	// Code
	s = regexp.MustCompile("`([^`]+)`").ReplaceAllString(s, "<code>$1</code>")
	return s
}
