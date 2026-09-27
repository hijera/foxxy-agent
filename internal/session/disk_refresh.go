package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// fork(session-disk-refresh): one bundle, two processes.
//
// A session is a folder under the home, and nothing stops two processes from
// holding it live at once: an editor panel's `foxxycode http` keeps the chat it
// shows, and the Telegram gateway can /resume the same session into a chat.
// The turn lock is a file lock, so the two never run a turn at the same time,
// but each keeps the history it read in memory. Without a check, the process
// that ran its turn second re-read nothing, answered on a history missing the
// other one's turn, and its save put that shorter history back on disk.
//
// Two guards close it, both keyed on what this State last knew the file to be:
//   - a turn re-reads the bundle when messages.json changed since then
//     (refreshFromDiskIfChanged, taken under the turn lock);
//   - a save of a State whose history has not moved since then does not
//     rewrite a file somebody else changed (FileStore.Save): a mode switch in
//     a stale panel must not undo the other process's turn.
//
// "Changed" is read from the content, not only from the stamp. A file that is
// gone, or that holds a proper prefix of this State's history, is older than
// this State, not newer: an older build without these guards wrote its stale
// copy back, or somebody deleted the file. Then this State's history is the
// one worth keeping, and the save writes it back as it always did.
//
// A State that did change its history in memory - the direct
// /v1/chat/completions path replaces it with the client's before it locks -
// is left alone by both: its history is the one the caller asked for, and the
// last writer wins, as before.

// diskStamp is what messages.json looked like when this State last read it
// or a save wrote or confirmed it, with the message revisions the in-memory
// history had at that moment.
type diskStamp struct {
	set     bool
	exists  bool
	size    int64
	modTime time.Time
	rev     uint64
	editRev uint64
}

// statMessages describes the bundle's messages.json as a stamp without
// revisions. A missing file is a state of its own, so a history written later
// by another process still reads as a change.
func statMessages(dir string) diskStamp {
	st, err := os.Stat(filepath.Join(dir, messagesFile))
	if err != nil {
		return diskStamp{set: true}
	}
	return diskStamp{set: true, exists: true, size: st.Size(), modTime: st.ModTime()}
}

// sameFile reports whether two stamps describe the same file contents.
func (d diskStamp) sameFile(o diskStamp) bool {
	if d.exists != o.exists {
		return false
	}
	return !d.exists || (d.size == o.size && d.modTime.Equal(o.modTime))
}

// setDiskStamp records the file as seen, together with the revisions the
// history had when it was.
func (s *State) setDiskStamp(file diskStamp, rev, editRev uint64) {
	file.rev, file.editRev = rev, editRev
	s.mu.Lock()
	s.diskMsgs = file
	s.mu.Unlock()
}

// stampCurrentDisk records the file as seen together with the history's
// current revisions: the two describe the same conversation right now.
func (s *State) stampCurrentDisk(file diskStamp) {
	s.mu.Lock()
	file.rev, file.editRev = s.msgRev, s.msgEditRev
	s.diskMsgs = file
	s.mu.Unlock()
}

// staleAgainstDisk reports whether messages.json holds a newer conversation
// than this State: the file changed since this State last saw it, the history
// in memory stayed where it was then, and what the file holds now is not an
// older copy of it (diskHistoryIsNewer).
func (s *State) staleAgainstDisk(dir string, msgs []llm.Message, rev, editRev uint64) bool {
	s.mu.RLock()
	stamp := s.diskMsgs
	s.mu.RUnlock()
	if !stamp.set || stamp.rev != rev || stamp.editRev != editRev {
		return false
	}
	if stamp.sameFile(statMessages(dir)) {
		return false
	}
	return diskHistoryIsNewer(readMessagesAt(dir), msgs)
}

// readMessagesAt decodes a bundle's messages.json; nil when it is missing or
// unreadable.
func readMessagesAt(dir string) []llm.Message {
	b, err := readFileWithRetry(filepath.Join(dir, messagesFile))
	if err != nil {
		return nil
	}
	var wrap messagesFileData
	if json.Unmarshal(b, &wrap) != nil {
		return nil
	}
	return wrap.Messages
}

// diskHistoryIsNewer reports whether the history read from disk is newer than
// ours: anything except no history at all or a prefix of ours, which is where
// an older writer - or a deleted file - leaves the bundle.
func diskHistoryIsNewer(disk, ours []llm.Message) bool {
	if len(disk) == 0 || len(disk) > len(ours) {
		return len(disk) > 0
	}
	for i := range disk {
		a, errA := json.Marshal(disk[i])
		b, errB := json.Marshal(ours[i])
		if errA != nil || errB != nil || !bytes.Equal(a, b) {
			return true
		}
	}
	return false
}

// restorePersistedFields copies what a bundle stores into st without
// persisting anything: the history, the plan, the grants, the UI log and the
// meta fields a client can change. Loading a bundle and refreshing a live
// session from it share it, so the two cannot drift apart.
func restorePersistedFields(st *State, snap *LoadedSnapshot) {
	mode := Mode(snap.Meta.Mode)
	if !IsValidMode(string(mode)) {
		mode = ModeAgent
	}
	st.RestoreMetaWithoutPersist(mode, snap.Meta.SelectedModelID, snap.Meta.SelectedReasoning, snap.Meta.AgentMemory, snap.Meta.PermissionMode)
	st.SetTitlePinnedWithoutPersist(snap.Meta.TitlePinned)
	st.SetTagsWithoutPersist(snap.Meta.Tags)
	st.SetArchivedWithoutPersist(snap.Meta.Archived, snap.Meta.ArchivedAt)
	st.SetOriginWithoutPersist(snap.Meta.Origin)
	st.SetPinnedWithoutPersist(snap.Meta.Pinned, snap.Meta.PinnedAt, snap.Meta.PinnedRank)
	st.RestoreHookContextWithoutPersist(snap.Meta.HookContext)
	st.SetTitleAutoWithoutPersist(snap.Meta.TitleAuto)
	st.ReplaceMessagesWithoutPersist(snap.Messages)
	st.SetPlanWithoutPersist(snap.Plan)
	st.RestorePermissionGrantsWithoutPersist(snap.PermissionCommands, snap.PermissionWriteKeys, snap.PermissionHTTPKeys)
	st.RestoreUILogWithoutPersist(snap.UILog)
}

// refreshFromDiskIfChanged re-reads a live session whose messages.json another
// process changed since this State last saw it. It runs under the turn lock,
// so the other process is not in a turn and the file is not moving.
//
// The State is refreshed in place rather than replaced: the turn about to run
// holds this pointer, and so do the MCP clients, the sender and the queue a
// fresh State would not have.
func (m *Manager) refreshFromDiskIfChanged(st *State) {
	if m == nil || m.store == nil || st == nil || st.SessionDir == "" || st.Subagent() != nil {
		return
	}
	st.mu.RLock()
	stamp, rev, editRev := st.diskMsgs, st.msgRev, st.msgEditRev
	st.mu.RUnlock()
	if !stamp.set || stamp.rev != rev || stamp.editRev != editRev {
		return
	}
	// Stat before the read: a write racing the read then still reads as a
	// change the next time, instead of being stamped as already seen.
	file := statMessages(st.SessionDir)
	if stamp.sameFile(file) {
		return
	}
	snap, err := m.store.readSnapshotAt(st.SessionDir, st.ID)
	if err != nil {
		m.log.Warn("session changed on disk; re-reading it failed", "session", st.ID, "error", err)
		return
	}
	ours := st.GetMessages()
	if !diskHistoryIsNewer(snap.Messages, ours) {
		// The file is older than this State: its next save writes it back.
		return
	}
	before := len(ours)
	restorePersistedFields(st, snap)
	st.mu.Lock()
	st.activitySeq = max(st.activitySeq, snap.Meta.ActivitySeq)
	st.readActivitySeq = max(st.readActivitySeq, snap.Meta.ReadActivitySeq)
	st.mu.Unlock()
	st.stampCurrentDisk(file)
	restoreContextBreakdown(st)
	m.sendContextUsageUpdate(st.ID, st)
	m.log.Info("session changed on disk by another process; re-read it before the turn",
		"session", st.ID, "messages_before", before, "messages", len(snap.Messages))
}
