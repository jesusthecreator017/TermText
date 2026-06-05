package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jesusthecreator017/TermText/internal/client"
	"github.com/jesusthecreator017/TermText/internal/crypto"
	"github.com/jesusthecreator017/TermText/internal/wsproto"
)

const (
	leftPaneWidth = 22
	historyPage   = 50
)

type wsEnvelopeMsg struct{ env wsproto.Envelope }
type wsErrorMsg struct{ err error }
type roomsLoadedMsg struct {
	rooms []client.Room
	err   error
}
type roomActionMsg struct {
	room client.Room
	err  error
}
type historyLoadedMsg struct {
	roomID string
	older  bool
	msgs   []client.Message
	err    error
}
type searchResultsMsg struct {
	query   string
	results []client.Message
	err     error
}
type publicRoomsLoadedMsg struct {
	rooms []client.Room
	err   error
}
type keyFetchedMsg struct {
	userID   string
	username string
	pub      []byte
	err      error
}

type bufMsg struct {
	id        string
	createdAt time.Time
	username  string
	body      string
}

// roomListItem adapts a room to the bubbles/list item interface.
type roomListItem struct{ room client.Room }

func (i roomListItem) Title() string {
	if i.room.Kind == "dm" {
		return "🔒 " + i.room.DisplayName
	}
	return "# " + i.room.DisplayName
}
func (i roomListItem) Description() string { return i.room.Kind }
func (i roomListItem) FilterValue() string { return i.room.DisplayName }

type roomBuffer struct {
	msgs    []bufMsg
	ids     map[string]bool
	loaded  bool
	loading bool
	hasMore bool
}

func newRoomBuffer() *roomBuffer {
	return &roomBuffer{ids: map[string]bool{}}
}

func (rb *roomBuffer) insert(m bufMsg) bool {
	if m.id != "" && rb.ids[m.id] {
		return false
	}
	if m.id != "" {
		rb.ids[m.id] = true
	}
	rb.msgs = append(rb.msgs, m)
	sort.SliceStable(rb.msgs, func(i, j int) bool {
		if rb.msgs[i].createdAt.Equal(rb.msgs[j].createdAt) {
			return rb.msgs[i].id < rb.msgs[j].id
		}
		return rb.msgs[i].createdAt.Before(rb.msgs[j].createdAt)
	})
	return true
}

func (rb *roomBuffer) oldestID() string {
	if len(rb.msgs) == 0 {
		return ""
	}
	return rb.msgs[0].id
}

type chatModel struct {
	client   *client.Client
	token    string
	conn     *client.Conn
	username string

	rooms   []client.Room
	active  string
	buffers map[string]*roomBuffer
	unread  map[string]bool

	viewport viewport.Model
	textarea textarea.Model
	ready    bool
	width    int
	height   int
	status   string
	notice   string

	switching bool
	roomList  list.Model

	browsing   bool
	browseList list.Model

	searching     bool
	searchResults []client.Message
	searchSel     int
	searchQuery   string

	online         map[string]bool                 // usernames currently online
	typers         map[string]map[string]time.Time // roomID -> username -> typing expiry
	reads          map[string]map[string]string    // roomID -> username -> last-read message id
	sentRead       map[string]string               // roomID -> last message id we acked
	lastTypingSent time.Time

	priv    *[crypto.KeySize]byte            // own private key; nil when locked
	pubkeys map[string]*[crypto.KeySize]byte // peer userID -> public key
	known   map[string]string                // TOFU: peer userID -> fingerprint

	styles styles
	keys   keyMap
	help   help.Model
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func newChatModel(c *client.Client, token string, conn *client.Conn, username string, priv *[crypto.KeySize]byte) chatModel {
	ta := textarea.New()
	ta.Placeholder = "Message, or /room /dm /join /browse /search …"
	ta.Prompt = "┃ "
	ta.CharLimit = 2000
	ta.SetHeight(2)
	ta.ShowLineNumbers = false
	ta.Focus()

	rl := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	rl.Title = "Switch room"
	rl.SetStatusBarItemName("room", "rooms")

	bl := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	bl.Title = "Browse public rooms"
	bl.SetStatusBarItemName("room", "rooms")

	return chatModel{
		client:   c,
		token:    token,
		conn:     conn,
		username: username,
		priv:     priv,
		buffers:  map[string]*roomBuffer{},
		unread:   map[string]bool{},
		online:   map[string]bool{},
		typers:   map[string]map[string]time.Time{},
		reads:    map[string]map[string]string{},
		sentRead: map[string]string{},
		pubkeys:  map[string]*[crypto.KeySize]byte{},
		known:    client.LoadKnownKeys(),
		textarea:   ta,
		roomList:   rl,
		browseList: bl,
		status:     "connected",
		styles:   newStyles(),
		keys:     defaultKeys(),
		help:     help.New(),
	}
}

func (m chatModel) initCmd() tea.Cmd {
	return tea.Batch(blinkCmd(), tickCmd(), loadRoomsCmd(m.client, m.token))
}

func blinkCmd() tea.Cmd { return textarea.Blink }

func readCmd(conn *client.Conn) tea.Cmd {
	return func() tea.Msg {
		env, err := conn.Read(context.Background())
		if err != nil {
			return wsErrorMsg{err}
		}
		return wsEnvelopeMsg{env}
	}
}

func loadRoomsCmd(c *client.Client, token string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rooms, err := c.ListRooms(ctx, token)
		return roomsLoadedMsg{rooms: rooms, err: err}
	}
}

func createRoomCmd(c *client.Client, token, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		room, err := c.CreateRoom(ctx, token, name)
		return roomActionMsg{room: room, err: err}
	}
}

func createDMCmd(c *client.Client, token, username string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		room, err := c.CreateDM(ctx, token, username)
		return roomActionMsg{room: room, err: err}
	}
}

func joinRoomCmd(c *client.Client, token, ident string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		room, err := c.JoinRoom(ctx, token, ident)
		return roomActionMsg{room: room, err: err}
	}
}

func loadPublicRoomsCmd(c *client.Client, token string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rooms, err := c.ListPublicRooms(ctx, token)
		return publicRoomsLoadedMsg{rooms: rooms, err: err}
	}
}

func loadHistoryCmd(c *client.Client, token, roomID, before string, older bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		msgs, err := c.RoomMessages(ctx, token, roomID, before, historyPage)
		return historyLoadedMsg{roomID: roomID, older: older, msgs: msgs, err: err}
	}
}

func searchCmd(c *client.Client, token, query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		results, err := c.Search(ctx, token, query)
		return searchResultsMsg{query: query, results: results, err: err}
	}
}

func typingCmd(conn *client.Conn, roomID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.SendTyping(ctx, roomID)
		return nil
	}
}

func readCmdFor(conn *client.Conn, roomID, messageID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.SendRead(ctx, roomID, messageID)
		return nil
	}
}

func fetchKeyCmd(c *client.Client, token, userID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pub, username, err := c.GetPublicKey(ctx, token, userID)
		return keyFetchedMsg{userID: userID, username: username, pub: pub, err: err}
	}
}

func (m chatModel) update(msg tea.Msg) (chatModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		m.ready = true

	case roomsLoadedMsg:
		if msg.err != nil {
			m.status = "rooms error: " + msg.err.Error()
			return m, nil
		}
		m.rooms = msg.rooms
		if m.active == "" && len(m.rooms) > 0 {
			cmds = append(cmds, m.setActive(m.rooms[0].ID))
		}

	case roomActionMsg:
		if msg.err != nil {
			m.notice = msg.err.Error()
			return m, nil
		}
		m.upsertRoom(msg.room)
		cmds = append(cmds, m.setActive(msg.room.ID))
		m.notice = ""

	case historyLoadedMsg:
		m.applyHistory(msg)
		if cmd := m.maybeSendRead(); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tickMsg:
		m.pruneTypers()
		cmds = append(cmds, tickCmd())

	case keyFetchedMsg:
		m.applyFetchedKey(msg)

	case searchResultsMsg:
		if msg.err != nil {
			m.notice = "search: " + msg.err.Error()
			return m, nil
		}
		m.searching = true
		m.searchResults = msg.results
		m.searchQuery = msg.query
		m.searchSel = 0

	case publicRoomsLoadedMsg:
		if msg.err != nil {
			m.notice = "browse: " + msg.err.Error()
			return m, nil
		}
		m.openBrowse(msg.rooms)

	case wsEnvelopeMsg:
		m.appendEnvelope(msg.env)
		cmds = append(cmds, readCmd(m.conn))
		if cmd := m.maybeSendRead(); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case wsErrorMsg:
		m.status = "disconnected: " + msg.err.Error()

	case tea.KeyMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		if m.switching {
			return m.updateSwitcher(msg)
		}
		if m.browsing {
			return m.updateBrowse(msg)
		}
		switch msg.String() {
		case "ctrl+k":
			m.openSwitcher()
			return m, nil
		case "enter":
			return m, m.handleInput()
		default:
			if cmd := m.maybeLoadOlder(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
			if cmd := m.maybeSendTyping(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	if !m.switching && !m.searching {
		var tcmd, vcmd tea.Cmd
		m.textarea, tcmd = m.textarea.Update(msg)
		m.viewport, vcmd = m.viewport.Update(msg)
		cmds = append(cmds, tcmd, vcmd)
	}
	return m, tea.Batch(cmds...)
}

// maybeLoadOlder triggers a previous-page fetch when the user scrolls to the top.
func (m *chatModel) maybeLoadOlder(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k", "pgup", "ctrl+b", "ctrl+u":
	default:
		return nil
	}
	if !m.viewport.AtTop() {
		return nil
	}
	rb := m.buffers[m.active]
	if rb == nil || rb.loading || !rb.hasMore || rb.oldestID() == "" {
		return nil
	}
	rb.loading = true
	return loadHistoryCmd(m.client, m.token, m.active, rb.oldestID(), true)
}

// maybeSendTyping emits a typing event at most once per second while editing.
func (m *chatModel) maybeSendTyping(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyRunes, tea.KeySpace, tea.KeyBackspace:
	default:
		return nil
	}
	if m.active == "" || time.Since(m.lastTypingSent) < time.Second {
		return nil
	}
	m.lastTypingSent = time.Now()
	return typingCmd(m.conn, m.active)
}

// maybeSendRead acks the newest message in the active room when at the bottom.
func (m *chatModel) maybeSendRead() tea.Cmd {
	if m.active == "" || !m.viewport.AtBottom() {
		return nil
	}
	rb := m.buffers[m.active]
	if rb == nil || len(rb.msgs) == 0 {
		return nil
	}
	latest := rb.msgs[len(rb.msgs)-1].id
	if latest == "" || m.sentRead[m.active] == latest {
		return nil
	}
	m.sentRead[m.active] = latest
	return readCmdFor(m.conn, m.active, latest)
}

func (m *chatModel) applyHistory(msg historyLoadedMsg) {
	rb := m.ensureBuffer(msg.roomID)
	rb.loading = false
	if msg.err != nil {
		m.notice = "history: " + msg.err.Error()
		return
	}
	for _, mm := range msg.msgs {
		rb.insert(rawBufMsg(mm.ID, mm.Username, mm.Body, mm.SentAt))
	}
	rb.loaded = true
	rb.hasMore = len(msg.msgs) == historyPage
	if msg.roomID == m.active {
		m.viewport.SetContent(m.renderBuffer(msg.roomID, rb))
		if !msg.older {
			m.viewport.GotoBottom()
		}
	}
}

func (m *chatModel) applyFetchedKey(msg keyFetchedMsg) {
	if msg.err != nil || len(msg.pub) != crypto.KeySize {
		m.notice = "no encryption key for that user yet"
		return
	}
	var pub [crypto.KeySize]byte
	copy(pub[:], msg.pub)
	m.pubkeys[msg.userID] = &pub

	fp := crypto.Fingerprint(&pub)
	if old, ok := m.known[msg.userID]; ok && old != fp {
		m.notice = "⚠ key changed for " + msg.username + " — possible MITM"
	}
	m.known[msg.userID] = fp
	_ = client.SaveKnownKey(msg.userID, fp)

	if rb := m.buffers[m.active]; rb != nil && m.ready {
		m.viewport.SetContent(m.renderBuffer(m.active, rb))
	}
}

func (m *chatModel) pruneTypers() {
	now := time.Now()
	for room, users := range m.typers {
		for u, exp := range users {
			if exp.Before(now) {
				delete(users, u)
			}
		}
		if len(users) == 0 {
			delete(m.typers, room)
		}
	}
}

// activeTypers returns the non-expired typists in the active room.
func (m chatModel) activeTypers() []string {
	now := time.Now()
	var names []string
	for u, exp := range m.typers[m.active] {
		if exp.After(now) {
			names = append(names, u)
		}
	}
	sort.Strings(names)
	return names
}

// seenBy counts other members who have read the newest message in the active room.
func (m chatModel) seenBy() int {
	rb := m.buffers[m.active]
	if rb == nil || len(rb.msgs) == 0 {
		return 0
	}
	latest := rb.msgs[len(rb.msgs)-1].id
	n := 0
	for u, mid := range m.reads[m.active] {
		if u != m.username && mid == latest {
			n++
		}
	}
	return n
}

func (m *chatModel) handleInput() tea.Cmd {
	body := strings.TrimSpace(m.textarea.Value())
	if body == "" {
		return nil
	}
	m.textarea.Reset()
	m.notice = ""

	if strings.HasPrefix(body, "/") {
		return m.handleCommand(body)
	}
	if m.active == "" {
		m.notice = "no room selected — use ctrl+k or /room <name>"
		return nil
	}
	return m.sendToActive(body)
}

// sendToActive sends plaintext to non-DM rooms and sealed ciphertext to DMs.
func (m *chatModel) sendToActive(plaintext string) tea.Cmd {
	room, isDM := m.isDM(m.active)
	if !isDM {
		return m.sendCmd(m.active, plaintext)
	}
	if m.priv == nil {
		m.notice = "🔒 locked — sign out (ctrl+l) and log in with your password to send DMs"
		return nil
	}
	peerPub := m.pubkeys[room.PeerID]
	if peerPub == nil {
		m.notice = "fetching encryption key… try again in a moment"
		return m.ensurePeerKey(m.active)
	}
	sealed, err := crypto.Seal(plaintext, peerPub, m.priv)
	if err != nil {
		m.notice = "encryption failed"
		return nil
	}
	return m.sendCmd(m.active, sealed)
}

func (m *chatModel) handleCommand(line string) tea.Cmd {
	fields := strings.Fields(line)
	cmd := fields[0]
	arg := strings.TrimSpace(strings.TrimPrefix(line, cmd))
	switch cmd {
	case "/room":
		if arg == "" {
			m.notice = "usage: /room <name>"
			return nil
		}
		return createRoomCmd(m.client, m.token, arg)
	case "/dm":
		if arg == "" {
			m.notice = "usage: /dm <username>"
			return nil
		}
		return createDMCmd(m.client, m.token, arg)
	case "/join":
		if arg == "" {
			m.notice = "usage: /join <room name>"
			return nil
		}
		return joinRoomCmd(m.client, m.token, arg)
	case "/browse":
		return loadPublicRoomsCmd(m.client, m.token)
	case "/search":
		if arg == "" {
			m.notice = "usage: /search <text>"
			return nil
		}
		return searchCmd(m.client, m.token, arg)
	case "/help":
		m.notice = "/room · /dm · /join · /browse · /search · ctrl+k switch · ctrl+l sign out"
		return nil
	default:
		m.notice = "unknown command: " + cmd
		return nil
	}
}

func (m chatModel) sendCmd(roomID, body string) tea.Cmd {
	conn := m.conn
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.Send(ctx, roomID, body); err != nil {
			return wsErrorMsg{err}
		}
		return nil
	}
}

func (m *chatModel) openSwitcher() {
	items := make([]list.Item, len(m.rooms))
	for i, r := range m.rooms {
		items[i] = roomListItem{room: r}
	}
	m.roomList.SetItems(items)
	m.roomList.ResetFilter()
	if m.width > 0 && m.height > 0 {
		m.roomList.SetSize(m.width, m.height)
	}
	m.switching = true
}

func (m chatModel) updateSwitcher(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	// While typing a filter, let the list own all keys (incl. esc/enter).
	if m.roomList.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.roomList, cmd = m.roomList.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "esc", "ctrl+k":
		m.switching = false
		return m, nil
	case "enter":
		var cmd tea.Cmd
		if it, ok := m.roomList.SelectedItem().(roomListItem); ok {
			cmd = m.setActive(it.room.ID)
		}
		m.switching = false
		return m, cmd
	}
	var cmd tea.Cmd
	m.roomList, cmd = m.roomList.Update(msg)
	return m, cmd
}

func (m *chatModel) openBrowse(rooms []client.Room) {
	items := make([]list.Item, len(rooms))
	for i, r := range rooms {
		items[i] = roomListItem{room: r}
	}
	m.browseList.SetItems(items)
	m.browseList.ResetFilter()
	if m.width > 0 && m.height > 0 {
		m.browseList.SetSize(m.width, m.height)
	}
	m.browsing = true
}

func (m chatModel) updateBrowse(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	if m.browseList.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.browseList, cmd = m.browseList.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "esc":
		m.browsing = false
		return m, nil
	case "enter":
		var cmd tea.Cmd
		if it, ok := m.browseList.SelectedItem().(roomListItem); ok {
			cmd = joinRoomCmd(m.client, m.token, it.room.ID) // roomActionMsg joins + switches
		}
		m.browsing = false
		return m, cmd
	}
	var cmd tea.Cmd
	m.browseList, cmd = m.browseList.Update(msg)
	return m, cmd
}

func (m chatModel) updateSearch(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		return m, nil
	case "up":
		if m.searchSel > 0 {
			m.searchSel--
		}
		return m, nil
	case "down":
		if m.searchSel < len(m.searchResults)-1 {
			m.searchSel++
		}
		return m, nil
	case "enter":
		var cmd tea.Cmd
		if m.searchSel < len(m.searchResults) {
			cmd = m.setActive(m.searchResults[m.searchSel].RoomID)
		}
		m.searching = false
		return m, cmd
	}
	return m, nil
}

func (m *chatModel) setActive(roomID string) tea.Cmd {
	m.active = roomID
	delete(m.unread, roomID)
	rb := m.ensureBuffer(roomID)
	if m.ready {
		m.viewport.SetContent(m.renderBuffer(roomID, rb))
		m.viewport.GotoBottom()
	}

	var cmds []tea.Cmd
	if cmd := m.ensurePeerKey(roomID); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if !rb.loaded && !rb.loading {
		rb.loading = true
		cmds = append(cmds, loadHistoryCmd(m.client, m.token, roomID, "", false))
	} else {
		cmds = append(cmds, m.maybeSendRead())
	}
	return tea.Batch(cmds...)
}

// ensurePeerKey fetches the DM partner's public key if not already cached.
func (m *chatModel) ensurePeerKey(roomID string) tea.Cmd {
	room, isDM := m.isDM(roomID)
	if !isDM || room.PeerID == "" || m.pubkeys[room.PeerID] != nil {
		return nil
	}
	return fetchKeyCmd(m.client, m.token, room.PeerID)
}

func (m *chatModel) ensureBuffer(roomID string) *roomBuffer {
	rb := m.buffers[roomID]
	if rb == nil {
		rb = newRoomBuffer()
		m.buffers[roomID] = rb
	}
	return rb
}

func (m *chatModel) upsertRoom(room client.Room) {
	for i, r := range m.rooms {
		if r.ID == room.ID {
			m.rooms[i] = room
			return
		}
	}
	m.rooms = append(m.rooms, room)
}

func (m chatModel) isDM(roomID string) (client.Room, bool) {
	for _, r := range m.rooms {
		if r.ID == roomID {
			return r, r.Kind == "dm"
		}
	}
	return client.Room{}, false
}

func (m *chatModel) layout() {
	taHeight := 3
	headerHeight := 2
	footerHeight := 1
	vpHeight := max(m.height-taHeight-headerHeight-footerHeight, 1)
	vpWidth := max(m.width-leftPaneWidth-1, 1)

	if !m.ready {
		m.viewport = viewport.New(vpWidth, vpHeight)
	} else {
		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
	}
	m.textarea.SetWidth(vpWidth)
	m.help.Width = vpWidth
	m.roomList.SetSize(m.width, m.height)
	m.browseList.SetSize(m.width, m.height)
	if rb := m.buffers[m.active]; rb != nil {
		m.viewport.SetContent(m.renderBuffer(m.active, rb))
	}
}

func (m *chatModel) appendEnvelope(env wsproto.Envelope) {
	switch env.Type {
	case wsproto.TypeMessage:
		var p wsproto.MessagePayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return
		}
		rb := m.ensureBuffer(p.RoomID)
		if !rb.insert(rawBufMsg(p.ID, p.Username, p.Body, p.SentAt)) {
			return
		}
		if p.RoomID == m.active {
			m.viewport.SetContent(m.renderBuffer(m.active, rb))
			m.viewport.GotoBottom()
		} else {
			m.unread[p.RoomID] = true
		}
	case wsproto.TypePresence:
		var p wsproto.PresencePayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.Username == "" {
			return
		}
		if p.Online {
			m.online[p.Username] = true
		} else {
			delete(m.online, p.Username)
		}
	case wsproto.TypeTyping:
		var p wsproto.TypingPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil || p.Username == m.username {
			return
		}
		if m.typers[p.RoomID] == nil {
			m.typers[p.RoomID] = map[string]time.Time{}
		}
		m.typers[p.RoomID][p.Username] = time.Now().Add(3 * time.Second)
	case wsproto.TypeRead:
		var p wsproto.ReadPayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return
		}
		if m.reads[p.RoomID] == nil {
			m.reads[p.RoomID] = map[string]string{}
		}
		m.reads[p.RoomID][p.Username] = p.MessageID
	case wsproto.TypeError:
		var p wsproto.ErrorPayload
		if err := json.Unmarshal(env.Payload, &p); err == nil {
			m.notice = "! " + p.Message
		}
	}
}

// renderBuffer formats a room's messages, decrypting DM bodies at render time so
// they update once the peer key arrives.
func (m chatModel) renderBuffer(roomID string, rb *roomBuffer) string {
	lines := make([]string, len(rb.msgs))
	for i, msg := range rb.msgs {
		lines[i] = m.formatLine(roomID, msg)
	}
	return strings.Join(lines, "\n")
}

func (m chatModel) formatLine(roomID string, msg bufMsg) string {
	ts := ""
	if !msg.createdAt.IsZero() {
		ts = msg.createdAt.Local().Format("15:04")
	}
	nameStyle := m.styles.otherName
	if msg.username == m.username {
		nameStyle = m.styles.selfName
	}
	body := m.decryptBody(roomID, msg.body)
	return fmt.Sprintf("%s %s  %s", m.styles.timestamp.Render(ts), nameStyle.Render(msg.username), body)
}

// decryptBody returns the display text for a message body. DM bodies are sealed;
// both parties decrypt with (peerPub, myPriv) thanks to the shared DH secret.
func (m chatModel) decryptBody(roomID, body string) string {
	room, isDM := m.isDM(roomID)
	if !isDM || !crypto.IsEncrypted(body) {
		return body
	}
	if m.priv == nil {
		return m.styles.lock.Render("🔒 locked — sign out & log in with password")
	}
	peerPub := m.pubkeys[room.PeerID]
	if peerPub == nil {
		return m.styles.lock.Render("🔒 …")
	}
	plain, ok := crypto.Open(body, peerPub, m.priv)
	if !ok {
		return m.styles.errorText.Render("🔒 (cannot decrypt)")
	}
	return plain
}

func rawBufMsg(id, username, body, sentAt string) bufMsg {
	var created time.Time
	if t, err := time.Parse(time.RFC3339, sentAt); err == nil {
		created = t
	}
	return bufMsg{id: id, createdAt: created, username: username, body: body}
}

func typingText(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0] + " is typing…"
	case 2:
		return names[0] + " and " + names[1] + " are typing…"
	default:
		return "several people are typing…"
	}
}

func (m chatModel) view() string {
	if !m.ready {
		return "initializing…"
	}
	if m.searching {
		return m.searchView()
	}
	if m.switching {
		return m.roomList.View()
	}
	if m.browsing {
		return m.browseList.View()
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.roomsView(), m.chatPane())
}

func (m chatModel) roomsView() string {
	var b strings.Builder
	b.WriteString(m.styles.title.Render("Rooms"))
	b.WriteString("\n\n")
	for _, r := range m.rooms {
		prefix := "# "
		if r.Kind == "dm" {
			prefix = "@ "
			if m.online[r.DisplayName] {
				prefix = "● "
			}
		}
		label := prefix + r.DisplayName
		if m.unread[r.ID] {
			label = "*" + label
		}
		style := m.styles.roomItem
		if r.ID == m.active {
			style = m.styles.roomActive
		}
		b.WriteString(style.Render(truncate(label, leftPaneWidth-2)) + "\n")
	}
	return m.styles.roomsPane.Height(m.height).Render(b.String())
}

func (m chatModel) chatPane() string {
	roomName := "—"
	for _, r := range m.rooms {
		if r.ID == m.active {
			roomName = r.DisplayName
			if r.Kind == "dm" {
				roomName = "🔒 " + roomName
			}
			break
		}
	}
	headerText := fmt.Sprintf("%s  ·  %s  ·  %s", roomName, m.username, m.status)
	if n := m.seenBy(); n > 0 {
		headerText += fmt.Sprintf("  ·  ✓ seen by %d", n)
	}
	header := m.styles.header.Render(headerText)

	var footer string
	switch {
	case len(m.activeTypers()) > 0:
		footer = m.styles.dim.Render(typingText(m.activeTypers()))
	case m.notice != "":
		footer = m.styles.warn.Render(m.notice)
	default:
		footer = m.help.ShortHelpView(m.keys.chatHelp())
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.viewport.View(),
		m.textarea.View(),
		footer,
	)
}

func (m chatModel) searchView() string {
	var b strings.Builder
	b.WriteString(m.styles.title.Render(fmt.Sprintf("Search results for %q", m.searchQuery)) + "\n\n")
	if len(m.searchResults) == 0 {
		b.WriteString(m.styles.dim.Render("no matches"))
	}
	for i, res := range m.searchResults {
		prefix := "  "
		style := m.styles.roomItem
		if i == m.searchSel {
			prefix = "› "
			style = m.styles.selected
		}
		room := m.roomName(res.RoomID)
		line := fmt.Sprintf("%s[%s] %s: %s", prefix, room, res.Username, truncate(res.Body, m.width-30))
		b.WriteString(style.Render(line) + "\n")
	}
	b.WriteString("\n" + m.help.ShortHelpView(m.keys.overlayHelp()))
	return b.String()
}

func (m chatModel) roomName(id string) string {
	for _, r := range m.rooms {
		if r.ID == id {
			return r.DisplayName
		}
	}
	return "room"
}

func truncate(s string, n int) string {
	if n < 1 {
		n = 1
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
