package lib

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type RoomAuthRequirement struct {
	Room     string
	Required bool
	Password string
}

func normalizeAuthValue(auth interface{}) string {
	switch value := auth.(type) {
	case string:
		return value
	case int:
		if value != 0 {
			return strconv.Itoa(value)
		}
	case float64:
		if value != 0 {
			return strconv.FormatFloat(value, 'f', 0, 64)
		}
	case json.Number:
		return string(value)
	}

	return ""
}

func normalizeRoomAuthConfig(roomAuth map[string]string) map[string]string {
	if len(roomAuth) == 0 {
		return map[string]string{}
	}

	normalized := make(map[string]string, len(roomAuth))
	for room, password := range roomAuth {
		normalized[normalizeRoomName(room)] = password
	}

	return normalized
}

func extractAuthToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return parts[1]
		}
		return authHeader
	}

	return r.URL.Query().Get("auth")
}

func extractAuthTokens(r *http.Request) []string {
	tokens := []string{}
	pushToken := func(token string) {
		normalized := strings.TrimSpace(token)
		if normalized == "" {
			return
		}
		for _, existing := range tokens {
			if existing == normalized {
				return
			}
		}
		tokens = append(tokens, normalized)
	}

	pushToken(extractAuthToken(r))

	extraHeader := strings.TrimSpace(r.Header.Get("X-Room-Auth-Tokens"))
	if extraHeader == "" {
		return tokens
	}

	var parsed []string
	if err := json.Unmarshal([]byte(extraHeader), &parsed); err == nil {
		for _, token := range parsed {
			pushToken(token)
		}
		return tokens
	}

	for _, token := range strings.Split(extraHeader, ",") {
		pushToken(token)
	}

	return tokens
}

func (s *ClipboardServer) resolveRoomAuth(room string) RoomAuthRequirement {
	normalizedRoom := normalizeRoomName(room)
	globalPassword := normalizeAuthValue(s.config.Server.Auth)
	roomPassword, hasRoomPassword := s.config.Server.RoomAuth[normalizedRoom]

	if roomPassword != "" {
		return RoomAuthRequirement{Room: normalizedRoom, Required: true, Password: roomPassword}
	}

	if globalPassword != "" {
		return RoomAuthRequirement{Room: normalizedRoom, Required: true, Password: globalPassword}
	}

	if hasRoomPassword {
		return RoomAuthRequirement{Room: normalizedRoom}
	}

	return RoomAuthRequirement{Room: normalizedRoom}
}

func (s *ClipboardServer) tokenMatchesRoom(room string, token string) bool {
	if token == "" {
		return false
	}

	globalPassword := normalizeAuthValue(s.config.Server.Auth)
	if globalPassword != "" && token == globalPassword {
		return true
	}

	normalizedRoom := normalizeRoomName(room)
	if roomPassword, ok := s.config.Server.RoomAuth[normalizedRoom]; ok && roomPassword != "" {
		return token == roomPassword
	}

	return false
}

func (s *ClipboardServer) canAccessRoom(room string, token string) bool {
	requirement := s.resolveRoomAuth(room)
	if !requirement.Required {
		return true
	}

	return s.tokenMatchesRoom(room, token)
}

func (s *ClipboardServer) hasRoomAuthEntry(room string) bool {
	normalizedRoom := normalizeRoomName(room)
	_, ok := s.config.Server.RoomAuth[normalizedRoom]
	return ok

}

func (s *ClipboardServer) getUploadedFileRoom(uuid string) (string, bool) {
	s.runMutex.Lock()
	defer s.runMutex.Unlock()

	fileInfo, ok := s.uploadFileMap[uuid]
	if !ok {
		return "", false
	}

	return normalizeRoomName(fileInfo.Room), true
}

func (s *ClipboardServer) inferRequestRoom(r *http.Request) string {
	if _, hasRoom := r.URL.Query()["room"]; hasRoom {
		return normalizeRoomName(r.URL.Query().Get("room"))
	}

	filePrefix := s.config.Server.Prefix + "/file/"
	chunkPrefix := s.config.Server.Prefix + "/upload/chunk/"
	finishPrefix := s.config.Server.Prefix + "/upload/finish/"

	var uuid string
	switch {
	case strings.HasPrefix(r.URL.Path, filePrefix):
		pathPart := strings.TrimPrefix(r.URL.Path, filePrefix)
		uuid = strings.SplitN(pathPart, "/", 2)[0]
	case strings.HasPrefix(r.URL.Path, chunkPrefix):
		uuid = strings.TrimPrefix(r.URL.Path, chunkPrefix)
	case strings.HasPrefix(r.URL.Path, finishPrefix):
		uuid = strings.TrimPrefix(r.URL.Path, finishPrefix)
	}

	if uuid != "" {
		if room, ok := s.getUploadedFileRoom(uuid); ok {
			return room
		}
	}

	return "default"
}

func writeAuthJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   http.StatusText(status),
		"message": message,
	})
}
