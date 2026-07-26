package connectors

import (
	"sort"
	"strings"
)

const (
	GoogleModuleGmail    = "gmail"
	GoogleModuleCalendar = "calendar"
	GoogleModuleDrive    = "drive"
	GoogleModuleTasks    = "tasks"
	GoogleModuleChat     = "chat"
)

var googleModuleOrder = []string{
	GoogleModuleGmail,
	GoogleModuleCalendar,
	GoogleModuleDrive,
	GoogleModuleTasks,
	GoogleModuleChat,
}

func GoogleModules(value string) []string {
	allowed := map[string]bool{}
	for _, module := range googleModuleOrder {
		allowed[module] = true
	}
	seen := map[string]bool{}
	for _, module := range strings.Split(value, ",") {
		module = strings.ToLower(strings.TrimSpace(module))
		if allowed[module] {
			seen[module] = true
		}
	}
	result := make([]string, 0, len(seen))
	for _, module := range googleModuleOrder {
		if seen[module] {
			result = append(result, module)
		}
	}
	return result
}

func GoogleModulesValue(modules []string) string {
	return strings.Join(GoogleModules(strings.Join(modules, ",")), ",")
}

func GoogleScopes(modules []string) []string {
	result := []string{"openid", "email"}
	selected := map[string]bool{}
	for _, module := range GoogleModules(strings.Join(modules, ",")) {
		selected[module] = true
	}
	if selected[GoogleModuleGmail] {
		result = append(result, "https://www.googleapis.com/auth/gmail.readonly")
	}
	if selected[GoogleModuleCalendar] {
		result = append(result, "https://www.googleapis.com/auth/calendar.readonly")
	}
	if selected[GoogleModuleDrive] {
		result = append(result, "https://www.googleapis.com/auth/drive.activity.readonly")
	}
	if selected[GoogleModuleTasks] {
		result = append(result, "https://www.googleapis.com/auth/tasks.readonly")
	}
	if selected[GoogleModuleChat] {
		result = append(result,
			"https://www.googleapis.com/auth/chat.spaces.readonly",
			"https://www.googleapis.com/auth/chat.messages.readonly",
		)
	}
	return result
}

func ValidGoogleModules(value string) bool {
	raw := strings.Split(value, ",")
	if len(raw) == 0 {
		return false
	}
	valid := GoogleModules(value)
	if len(valid) == 0 {
		return false
	}
	normalizedRaw := make([]string, 0, len(raw))
	for _, module := range raw {
		module = strings.ToLower(strings.TrimSpace(module))
		if module != "" {
			normalizedRaw = append(normalizedRaw, module)
		}
	}
	sort.Strings(normalizedRaw)
	sortedValid := append([]string(nil), valid...)
	sort.Strings(sortedValid)
	return strings.Join(normalizedRaw, ",") == strings.Join(sortedValid, ",")
}
