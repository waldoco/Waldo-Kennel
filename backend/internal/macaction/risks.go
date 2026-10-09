package macaction

import (
	"path/filepath"
	"regexp"
	"strings"
)

var pipeShell = regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n]*\|\s*(?:/[^ ]*/)?(?:ba|z|fi|da)?sh\b`)
var writeCommand = regexp.MustCompile(`\b(?:cp|mv|tee)\s+[^;\n|&]+`)
var shellWord = regexp.MustCompile(`"[^"\n]*"|'[^'\n]*'|[^\s]+`)
var outside = regexp.MustCompile(`(?:>|>>|\b(?:cp|mv|tee)\s+)\s*(/[^\s;]+|\.\./[^\s;]+)`)

func Risks(text, folder string) []string {
	flags := []string{}
	lower := strings.ToLower(text)
	if regexp.MustCompile(`\bsudo\b`).MatchString(lower) {
		flags = append(flags, "sudo")
	}
	if regexp.MustCompile(`\brm\s+[^\n;]*(-rf|-fr|--recursive)`).MatchString(lower) {
		flags = append(flags, "recursive deletion")
	}
	if pipeShell.MatchString(text) {
		flags = append(flags, "download piped to shell")
	}
	if strings.Contains(lower, ".ssh") || strings.Contains(lower, ".aws") || strings.Contains(lower, ".config/gcloud") || strings.Contains(lower, "keychain") || strings.Contains(lower, ".env") {
		flags = append(flags, "credential path")
	}
	paths := []string{}
	for _, m := range outside.FindAllStringSubmatch(text, -1) {
		paths = append(paths, m[1])
	}
	// Destination operands can follow sources and flags. These are warning
	// heuristics only; exact native approval remains the authorization boundary.
	for _, command := range writeCommand.FindAllString(text, -1) {
		words := shellWord.FindAllString(command, -1)
		for _, word := range words[1:] {
			word = strings.Trim(word, "\"'")
			if strings.HasPrefix(word, "/") || strings.HasPrefix(word, "../") {
				paths = append(paths, word)
			}
		}
	}
	for _, path := range paths {
		path = filepath.Clean(path)
		if strings.HasPrefix(path, "../") || !(path == folder || strings.HasPrefix(path, folder+"/")) {
			flags = append(flags, "write outside project")
			break
		}
	}
	return flags
}
