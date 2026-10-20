package crushfling

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultCaptionTemplate seeds crushfling/data/caption.txt on first run.
// Lines starting with # are comments and are preserved but never used as a bio.
const DefaultCaptionTemplate = `# CrushFling caption.txt — one line = one bio
# A line is deleted automatically only after its account is verified,
# so a failed attempt never burns a caption.
# @myhandle is replaced with the profile's platform username.
Hi I'm at home and alone. Send me a friend request 💋 I will send you my unlimited nudes, pussy, fingering videos, sextape, doggy style including live & FaceTime sexting: Add Telegram >> @myhandle If u not serious Don't come asking me for free content
Hey babe 😏 looking for fun tonight? I'm 100% real and verified. Message me on telegram: @myhandle 💦 No free pics without request
Single and ready to mingle 🌸 Love sexting and trading pics. Find me on telegram @myhandle - let's have fun!
Just here for naughty chats 😈 No time wasters pls. DM me on telegram: @myhandle if you're serious 🔥
Bored and horny 😽 wanna sext? Add my telegram @myhandle - I reply fast! No free content without request
Looking for a sexting partner who is respectful and fun 😊 My telegram is @myhandle - let's chat!
Open-minded girl looking for discreet fun 💋 telegram >> @myhandle | Let's trade and have a good time
I'm new here! Show me around? 😉 Add me: telegram @myhandle - I love sexting and video calls
Real girl, real nudes 📸 Add my telegram @myhandle if you want to see more. Serious only!
Let's get naughty together 🍑 My telegram is @myhandle - message me anytime babe 💕
Want you to moan and shout out your name bby ❣️ Add my telegram @myhandle - I'm waiting for you
The naughtiest thing I've done is I had a threesome in the car 🍆 Add telegram @myhandle for more
My ideal night looks like cuddling and sexting all night 🌙 telegram: @myhandle
I moan when I'm wet and fuck harder 💦 Add me @myhandle if you can handle me
I'll make you cum over again bby 🍆 My telegram is @myhandle - dare to try?
Young, wild and free 🌸 22yo woman looking for male. Add telegram @myhandle for sexting
Verified girl seeking male partner for discreet fun 😘 Telegram: @myhandle - reply in minutes
No fake, no scam - real sexting partner here 💋 Add telegram @myhandle and let's play
College girl looking for fun after class 📚 Telegram @myhandle - horny and ready
Shy at first but wild in sexting 😈 Add telegram @myhandle and I'll show you my naughty side
Love roleplay and dirty chat 💦 Telegram @myhandle - what's your fantasy bby?
Available now for sexting! 🔥 Add telegram @myhandle - I send unlimited nudes
Just a lonely girl looking for sexting partner 🌸 Telegram: @myhandle
Horny and alone at home 😽 Add my telegram @myhandle if you want to see my videos
Flirty girl wants to sext with respectful male 😊 Telegram >> @myhandle
Sexy, sassy and ready to play 😘 Telegram @myhandle - message me now
Looking for long-term sexting buddy 💕 Add telegram @myhandle - let's get to know each other
Naughty by nature 😈 Add telegram @myhandle if you're looking for fun
Sweet girl with wild side 🌸 Telegram: @myhandle - let's sext all night
Passionate lover seeking male companion 💋 Telegram @myhandle
Add me on telegram @myhandle - I promise you won't regret it 😘
`

// BioFileName is the preferred file name for the profile text list. The older
// caption.txt is still honoured, so an existing data folder keeps working.
const BioFileName = "bio.txt"

// BioPath returns the bio list that this data folder should use: bio.txt when
// it exists, otherwise caption.txt. bio.txt wins because the operator asked
// for that name; a data folder created by an older build only has
// caption.txt, and migrating silently would split the two lists.
func BioPath(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = filepath.Join("crushfling", "data")
	}
	bio := filepath.Join(dataDir, BioFileName)
	if st, err := os.Stat(bio); err == nil && !st.IsDir() {
		return bio
	}
	caption := filepath.Join(dataDir, "caption.txt")
	if st, err := os.Stat(caption); err == nil && !st.IsDir() {
		return caption
	}
	// Fresh folder: use the current name so a first run creates bio.txt.
	return bio
}

// BioInfo is what the dashboard shows about the active bio list.
type BioInfo struct {
	Path      string   `json:"path"`
	FileName  string   `json:"file_name"`
	Remaining int      `json:"remaining"`
	Total     int      `json:"total"`
	Preview   []string `json:"preview"`
}

// ReadBioInfo summarises a bio list without consuming anything.
func ReadBioInfo(dataDir string) (BioInfo, error) {
	path := BioPath(dataDir)
	cm, err := NewCaptionManager(path)
	if err != nil {
		return BioInfo{}, err
	}
	info := BioInfo{Path: path, FileName: filepath.Base(path), Remaining: cm.Count()}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, line := range cm.lines {
		if isCaptionComment(line) {
			continue
		}
		if len(info.Preview) < 8 {
			info.Preview = append(info.Preview, line)
		}
		info.Total++
	}
	return info, nil
}

// ImportBioFile copies a text file into <dataDir>/bio.txt, one bio per line.
// Blank lines and duplicates are dropped, "#" comments are kept out, and the
// file is written atomically so a failed import never truncates the old list.
// It is what the dashboard's "Load bio.txt" button calls.
func ImportBioFile(dataDir, srcPath string) (BioInfo, error) {
	srcPath = strings.TrimSpace(srcPath)
	if srcPath == "" {
		return BioInfo{}, fmt.Errorf("no bio file selected")
	}
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return BioInfo{}, fmt.Errorf("read bio file: %w", err)
	}
	lines, err := NormalizeBioLines(string(raw))
	if err != nil {
		return BioInfo{}, err
	}
	if strings.TrimSpace(dataDir) == "" {
		dataDir = filepath.Join("crushfling", "data")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return BioInfo{}, err
	}
	dest := filepath.Join(dataDir, BioFileName)
	tmp := dest + ".tmp"
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return BioInfo{}, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return BioInfo{}, err
	}
	return ReadBioInfo(dataDir)
}

// NormalizeBioLines cleans a bio file: trims each line, drops blanks,
// duplicates and comments, and keeps the order the operator wrote.
func NormalizeBioLines(raw string) ([]string, error) {
	raw = strings.TrimPrefix(raw, "\ufeff") // Windows editors may add a BOM
	seen := map[string]bool{}
	var out []string
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read bio file: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the bio file has no usable line")
	}
	return out, nil
}

// CaptionManager owns caption.txt. Every operation rewrites the file
// atomically so an interrupted run cannot corrupt the list.
type CaptionManager struct {
	path     string
	mu       sync.Mutex
	lines    []string        // includes comment lines; those are preserved verbatim
	reserved map[string]bool // lines handed to an in-flight account
}

// NewCaptionManager loads caption.txt, creating it from the template when the
// file does not exist yet.
func NewCaptionManager(captionPath string) (*CaptionManager, error) {
	if strings.TrimSpace(captionPath) == "" {
		captionPath = filepath.Join("crushfling", "data", "caption.txt")
	}
	cm := &CaptionManager{path: captionPath, reserved: map[string]bool{}}
	if _, err := os.Stat(captionPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(captionPath), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(captionPath, []byte(DefaultCaptionTemplate), 0o644); err != nil {
			return nil, err
		}
	}
	if err := cm.reload(); err != nil {
		return nil, err
	}
	return cm, nil
}

func (c *CaptionManager) reload() error {
	f, err := os.Open(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			c.lines = nil
			return nil
		}
		return err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	c.lines = lines
	return sc.Err()
}

func isCaptionComment(line string) bool {
	return strings.HasPrefix(line, "#")
}

// Count returns the number of usable (non-comment) captions left.
func (c *CaptionManager) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, line := range c.lines {
		if !isCaptionComment(line) {
			n++
		}
	}
	return n
}

// Next reserves the first caption that is not already in use by another
// account and returns it. A reservation does not touch the file: call
// RemoveLine after the account verifies, or Release if the attempt failed.
func (c *CaptionManager) Next() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reserved == nil {
		c.reserved = map[string]bool{}
	}
	for _, line := range c.lines {
		if isCaptionComment(line) || c.reserved[line] {
			continue
		}
		c.reserved[line] = true
		return line, nil
	}
	return "", fmt.Errorf("%s has no unused line left - add more lines", filepath.Base(c.path))
}

// Release gives a reservation back after a failed attempt.
func (c *CaptionManager) Release(caption string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reserved != nil {
		delete(c.reserved, caption)
	}
}

// Peek returns the next caption without removing it. The line is only deleted
// by RemoveLine after the registration that used it succeeded.
func (c *CaptionManager) Peek() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, line := range c.lines {
		if !isCaptionComment(line) {
			return line, nil
		}
	}
	return "", fmt.Errorf("%s has no usable lines - add one bio per line", filepath.Base(c.path))
}

// PickAndRemove takes the first caption and deletes it immediately. Kept for
// callers that do not have a verify step; RegisterOne uses Peek + RemoveLine.
func (c *CaptionManager) PickAndRemove() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, line := range c.lines {
		if isCaptionComment(line) {
			continue
		}
		c.lines = append(c.lines[:i], c.lines[i+1:]...)
		if err := c.saveLocked(); err != nil {
			return "", err
		}
		return line, nil
	}
	return "", fmt.Errorf("%s has no usable lines - add one bio per line", filepath.Base(c.path))
}

// RemoveLine deletes one specific caption after a successful registration.
func (c *CaptionManager) RemoveLine(caption string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reserved != nil {
		delete(c.reserved, caption)
	}
	for i, line := range c.lines {
		if line == caption && !isCaptionComment(line) {
			c.lines = append(c.lines[:i], c.lines[i+1:]...)
			return c.saveLocked()
		}
	}
	return nil
}

// Add appends one caption line.
func (c *CaptionManager) Add(caption string) error {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return fmt.Errorf("bio line is empty")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, caption)
	return c.saveLocked()
}

func (c *CaptionManager) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, line := range c.lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, c.path)
}

// PictureManager owns the avatar folder: a random picture is used once and
// then moved to pictures/used/ so it is never reused by accident.
type PictureManager struct {
	dir     string
	usedDir string
	mu      sync.Mutex
	files   []string
}

func NewPictureManager(picDir string) (*PictureManager, error) {
	if strings.TrimSpace(picDir) == "" {
		picDir = filepath.Join("crushfling", "data", "pictures")
	}
	pm := &PictureManager{dir: picDir, usedDir: filepath.Join(picDir, "used")}
	if err := os.MkdirAll(pm.usedDir, 0o755); err != nil {
		return nil, err
	}
	if err := pm.reload(); err != nil {
		return nil, err
	}
	return pm, nil
}

func (p *PictureManager) reload() error {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".jpg", ".jpeg", ".png", ".webp":
			files = append(files, filepath.Join(p.dir, e.Name()))
		}
	}
	p.files = files
	return nil
}

func (p *PictureManager) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.files)
}

// PickRandom returns a random picture path without moving it.
func (p *PictureManager) PickRandom() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.files) == 0 {
		return "", fmt.Errorf("no pictures in %s - add jpg/png/webp files", p.dir)
	}
	idx := 0
	if len(p.files) > 1 {
		idx = rand.Intn(len(p.files))
	}
	return p.files[idx], nil
}

// MarkUsed moves a picture into used/ after a successful upload.
func (p *PictureManager) MarkUsed(picPath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	dest := filepath.Join(p.usedDir, filepath.Base(picPath))
	if err := os.Rename(picPath, dest); err != nil {
		return err
	}
	for i, f := range p.files {
		if f == picPath {
			p.files = append(p.files[:i], p.files[i+1:]...)
			break
		}
	}
	return nil
}

// List returns the remaining picture paths.
func (p *PictureManager) List() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.files))
	copy(out, p.files)
	return out
}

// EnsureDataLayout creates the folders/files the feature needs and is safe to
// call repeatedly. It never overwrites an existing caption.txt.
func EnsureDataLayout(dataDir string) error {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = filepath.Join("crushfling", "data")
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "pictures", "used"), 0o755); err != nil {
		return err
	}
	// Fresh folders get bio.txt (the operator-facing name). A folder that
	// already has caption.txt keeps using it: BioPath prefers bio.txt only
	// when that file really exists, so we must not create an empty/duplicate
	// bio.txt next to an existing list.
	bioFile := filepath.Join(dataDir, BioFileName)
	captionFile := filepath.Join(dataDir, "caption.txt")
	_, bioErr := os.Stat(bioFile)
	_, captionErr := os.Stat(captionFile)
	if os.IsNotExist(bioErr) && os.IsNotExist(captionErr) {
		if err := os.WriteFile(bioFile, []byte(DefaultCaptionTemplate), 0o644); err != nil {
			return err
		}
	}
	if _, err := NewCaptionManager(BioPath(dataDir)); err != nil {
		return err
	}
	if err := ensureEndpointConfigFile(dataDir); err != nil {
		return err
	}
	readme := filepath.Join(dataDir, "pictures", "README.txt")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		_ = os.WriteFile(readme, []byte(picturesReadme), 0o644)
	}
	return nil
}

const picturesReadme = `Put your JPG/PNG/WEBP pictures here.

- One picture is used per account, then moved to used/.
- Recommended: 500x500 or 1080x1350, under 5 MB, single female portrait.
- Keep at least as many pictures as accounts you plan to create.
`
