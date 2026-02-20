package scripts

import (
	"embed"
	"io"
	"strings"
	"text/template"
)

//go:embed templates/*.sh templates/*.ps1
var scriptTemplates embed.FS

type ScriptData struct {
	Token     string
	ServerURL string
	Email     string
	Mode      string
	MachineID string
}

func GenerateScript(w io.Writer, osType string, data ScriptData) error {
	var templateName string
	switch osType {
	case "darwin":
		templateName = "templates/agent_darwin.sh"
	case "linux":
		templateName = "templates/agent_linux.sh"
	case "windows":
		templateName = "templates/agent_windows.ps1"
	default:
		// Default to Linux script for unknown Unix-like systems
		templateName = "templates/agent_linux.sh"
	}

	content, err := scriptTemplates.ReadFile(templateName)
	if err != nil {
		return err
	}

	tmpl, err := template.New("script").Parse(string(content))
	if err != nil {
		return err
	}

	return tmpl.Execute(w, data)
}

func DetectOS(userAgent string) string {
	ua := strings.ToLower(userAgent)
	if strings.Contains(ua, "windows") || strings.Contains(ua, "powershell") {
		return "windows"
	}
	if strings.Contains(ua, "darwin") || strings.Contains(ua, "mac") {
		return "darwin"
	}
	if strings.Contains(ua, "linux") {
		return "linux"
	}
	// Default to Linux for curl/wget without specific UA
	return "linux"
}
