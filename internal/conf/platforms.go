package conf

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPath is the livestream stack conf used by capture-router / gsr.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "livestream", "platforms.conf")
}

// Section holds one [platform] block.
type Section struct {
	Server       string
	Key          string
	VideoBitrate string
	AudioBitrate string
	// Extra preserves unknown keys (order not guaranteed).
	Extra map[string]string
}

// File is a parsed platforms.conf.
type File struct {
	Path     string
	Order    []string // section names in file order
	Sections map[string]*Section
	// preamble lines before first section
	Preamble []string
}

// Load reads an INI-like platforms.conf. Missing file → empty File.
func Load(path string) (*File, error) {
	f := &File{
		Path:     path,
		Sections: make(map[string]*Section),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return f, nil
		}
		return nil, err
	}
	var cur string
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, ";") {
			if cur == "" {
				f.Preamble = append(f.Preamble, line)
			}
			continue
		}
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			cur = strings.TrimSpace(trim[1 : len(trim)-1])
			if _, ok := f.Sections[cur]; !ok {
				f.Order = append(f.Order, cur)
				f.Sections[cur] = &Section{Extra: map[string]string{}}
			}
			continue
		}
		if cur == "" {
			f.Preamble = append(f.Preamble, line)
			continue
		}
		k, v, ok := strings.Cut(trim, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		sec := f.Sections[cur]
		switch k {
		case "server":
			sec.Server = v
		case "key":
			sec.Key = v
		case "video_bitrate":
			sec.VideoBitrate = v
		case "audio_bitrate":
			sec.AudioBitrate = v
		default:
			if sec.Extra == nil {
				sec.Extra = map[string]string{}
			}
			sec.Extra[k] = v
		}
	}
	return f, sc.Err()
}

// UpsertServerKey sets server/key for platform, preserving bitrates when present.
func (f *File) UpsertServerKey(platform, server, key, defaultVideo, defaultAudio string) {
	sec, ok := f.Sections[platform]
	if !ok {
		sec = &Section{
			VideoBitrate: defaultVideo,
			AudioBitrate: defaultAudio,
			Extra:        map[string]string{},
		}
		f.Sections[platform] = sec
		f.Order = append(f.Order, platform)
	}
	sec.Server = server
	sec.Key = key
	if sec.VideoBitrate == "" {
		sec.VideoBitrate = defaultVideo
	}
	if sec.AudioBitrate == "" {
		sec.AudioBitrate = defaultAudio
	}
}

// Write writes the conf back to f.Path (or path if set).
func (f *File) Write(path string) error {
	if path == "" {
		path = f.Path
	}
	if path == "" {
		return fmt.Errorf("conf: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, line := range f.Preamble {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if len(f.Preamble) > 0 {
		// blank line between preamble and sections if needed
		if last := f.Preamble[len(f.Preamble)-1]; strings.TrimSpace(last) != "" {
			b.WriteByte('\n')
		}
	}
	for i, name := range f.Order {
		sec := f.Sections[name]
		if sec == nil {
			continue
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "[%s]\n", name)
		fmt.Fprintf(&b, "server = %s\n", sec.Server)
		fmt.Fprintf(&b, "key = %s\n", sec.Key)
		if sec.VideoBitrate != "" {
			fmt.Fprintf(&b, "video_bitrate = %s\n", sec.VideoBitrate)
		}
		if sec.AudioBitrate != "" {
			fmt.Fprintf(&b, "audio_bitrate = %s\n", sec.AudioBitrate)
		}
		for k, v := range sec.Extra {
			fmt.Fprintf(&b, "%s = %s\n", k, v)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
