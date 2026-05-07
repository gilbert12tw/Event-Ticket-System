package ticketing

import "strings"

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func joinTags(tags []string) string {
	return strings.Join(normalizeTags(tags), ",")
}

func splitTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeTags(strings.Split(raw, ","))
}
