package yippy

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"log/slog"

	"github.com/google/uuid"
)

const (
	ChunkSize      = 1024 * 1024
	MaxBufferSize  = 5000 * 1024 * 1024
	CommandTimeout = 4 * time.Hour
	BufferTTL      = 4 * time.Hour
)

type SessionManager struct {
	sessions map[string]*TranscodeSession
	buffers  map[string]*TranscodeBuffer
	mu       sync.Mutex
	logger   *slog.Logger
}

type TranscodeSession struct {
	ID        string
	IPAddress string
	Buffer    *TranscodeBuffer
}

type TranscodeBuffer struct {
	FilePath       string
	Chunks         []*TranscodeChunk
	ChunksComplete int
	Finished       bool
	Mu             sync.RWMutex
	NotifyChans    map[string]chan int
	Cmd            *exec.Cmd
	CancelFunc     context.CancelFunc
	DestroyTicker  *time.Ticker
}

type TranscodeChunk struct {
	Bytes  []byte
	Offset int64
}

func NewSessionManager(logger *slog.Logger) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*TranscodeSession),
		buffers:  make(map[string]*TranscodeBuffer),
		logger:   logger,
	}
}

func (b *TranscodeBuffer) GetChunk(i int) []byte {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	if i < 0 || i >= len(b.Chunks) {
		return nil
	}

	return b.Chunks[i].Bytes
}

func (b *TranscodeBuffer) Len() int {
	b.Mu.RLock()
	l := len(b.Chunks)
	b.Mu.RUnlock()
	return l
}

func (b *TranscodeBuffer) IsFinished() bool {
	b.Mu.RLock()
	f := b.Finished
	b.Mu.RUnlock()
	return f
}

func (m *SessionManager) StartSession(ip, path string, ch chan int) (*TranscodeSession, error) {
	id := uuid.NewString()

	m.mu.Lock()
	buf, exists := m.buffers[path]
	m.mu.Unlock()

	var err error
	if !exists {
		buf, err = m.createBuffer(path)
		if err != nil {
			return nil, err
		}
	}

	m.mu.Lock()
	sess := &TranscodeSession{ID: id, IPAddress: ip, Buffer: buf}
	m.sessions[id] = sess
	m.mu.Unlock()

	buf.Mu.Lock()
	buf.NotifyChans[id] = ch
	chunksComplete := buf.ChunksComplete

	if buf.DestroyTicker != nil {
		buf.DestroyTicker.Stop()
		buf.DestroyTicker = nil
	}
	buf.Mu.Unlock()

	m.logger.Info("Started new session",
		"sessionID", id,
		"fileHandle", path,
		"chunksAvailable", chunksComplete)

	go func() {
		ch <- chunksComplete
	}()

	return sess, nil
}

func (m *SessionManager) createBuffer(path string) (*TranscodeBuffer, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}

	buf := &TranscodeBuffer{
		FilePath:    path,
		Chunks:      make([]*TranscodeChunk, 0),
		NotifyChans: make(map[string]chan int),
	}

	m.mu.Lock()
	if existingBuf, exists := m.buffers[path]; exists {
		m.mu.Unlock()
		return existingBuf, nil
	}

	m.buffers[path] = buf
	m.mu.Unlock()

	go m.startTranscoding(context.TODO(), buf)
	return buf, nil
}

func (m *SessionManager) StopSession(id string) {
	var buf *TranscodeBuffer
	var sessionExists bool

	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		buf = sess.Buffer
		delete(m.sessions, id)
		sessionExists = true
	}
	m.mu.Unlock()

	if sessionExists {
		buf.Mu.Lock()
		delete(buf.NotifyChans, id)
		buf.Mu.Unlock()

		m.CleanupBufferIfNeeded(buf.FilePath)
	}
}

func (m *SessionManager) GetOrCreateBuffer(path string) (*TranscodeBuffer, error) {
	m.mu.Lock()
	buf, exists := m.buffers[path]
	m.mu.Unlock()

	if exists {
		return buf, nil
	}

	return m.createBuffer(path)
}

func (m *SessionManager) startTranscoding(ctx context.Context, buf *TranscodeBuffer) {
	cmdCtx, cancel := context.WithTimeout(ctx, CommandTimeout)
	buf.CancelFunc = cancel

	cmd := exec.CommandContext(cmdCtx,
		"ffmpeg",
		"-i", buf.FilePath,
		"-map_metadata", "-1",
		"-map_chapters", "-1",
		"-threads", "0",
		"-map", "0:v:0",
		"-map", "0:a:0",
		"-codec:v", "libx264",
		"-preset", "medium",
		"-profile:v", "high",
		"-level", "4.1",
		"-crf", "12",
		"-pix_fmt", "yuv420p",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-frag_duration", "1000",
		"-codec:a", "aac",
		"-ac", "2",
		"-b:a", "384k",
		"-f", "mp4",
		"pipe:1",
	)
	buf.Cmd = cmd

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			m.logger.Debug("ffmpeg", sc.Text())
		}
	}()

	if err := cmd.Start(); err != nil {
		m.logger.Error("ffmpeg start", "error", err)

		buf.Mu.Lock()
		buf.Finished = true
		channels := make([]chan int, 0, len(buf.NotifyChans))
		for _, ch := range buf.NotifyChans {
			channels = append(channels, ch)
		}
		buf.NotifyChans = make(map[string]chan int)
		buf.Mu.Unlock()

		for _, ch := range channels {
			ch <- -1
		}
		return
	}

	tmp := make([]byte, ChunkSize)
	for {
		select {
		case <-ctx.Done():
			cmd.Process.Kill()
			return
		default:
			n, err := io.ReadAtLeast(stdout, tmp, 1)
			if err != nil {
				buf.Mu.Lock()
				buf.Finished = true
				count := buf.ChunksComplete
				channels := make([]chan int, 0, len(buf.NotifyChans))
				for _, ch := range buf.NotifyChans {
					channels = append(channels, ch)
				}
				buf.NotifyChans = make(map[string]chan int)
				buf.Mu.Unlock()

				for _, ch := range channels {
					ch <- count
				}
				return
			}

			data := make([]byte, n)
			copy(data, tmp[:n])

			var channels []chan int
			var count int

			buf.Mu.Lock()
			buf.Chunks = append(buf.Chunks, &TranscodeChunk{Bytes: data, Offset: int64(buf.ChunksComplete)})
			buf.ChunksComplete++
			count = buf.ChunksComplete

			channels = make([]chan int, 0, len(buf.NotifyChans))
			for _, ch := range buf.NotifyChans {
				channels = append(channels, ch)
			}
			buf.Mu.Unlock()

			for _, ch := range channels {
				ch <- count
			}
		}
	}
}

func (m *SessionManager) CleanupBufferIfNeeded(path string) {
	var needsCleanup bool
	var buf *TranscodeBuffer

	m.mu.Lock()
	for _, s := range m.sessions {
		if s.Buffer.FilePath == path {
			m.mu.Unlock()
			return
		}
	}

	buf, exists := m.buffers[path]
	if !exists {
		m.mu.Unlock()
		return
	}

	buf.Mu.RLock()
	needsCleanup = buf.DestroyTicker == nil
	buf.Mu.RUnlock()

	if !needsCleanup {
		m.mu.Unlock()
		return
	}

	ticker := time.NewTicker(BufferTTL)
	buf.Mu.Lock()
	buf.DestroyTicker = ticker
	buf.Mu.Unlock()
	m.mu.Unlock()

	go func() {
		<-ticker.C

		m.mu.Lock()
		currentBuf, exists := m.buffers[path]
		if !exists || currentBuf != buf {
			m.mu.Unlock()
			ticker.Stop()
			return
		}

		delete(m.buffers, path)
		m.mu.Unlock()

		buf.Mu.Lock()
		if buf.CancelFunc != nil {
			buf.CancelFunc()
		}
		if buf.DestroyTicker != nil {
			buf.DestroyTicker.Stop()
			buf.DestroyTicker = nil
		}
		buf.Mu.Unlock()
	}()
}
