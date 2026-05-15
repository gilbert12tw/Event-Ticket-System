package ticketing

import "strings"

// knownCities contains city names that can be detected from a location string.
// Order matters: longer/more-specific names should appear before shorter overlapping ones.
var knownCities = []string{
	"Kaohsiung",
	"Taichung",
	"Tainan",
	"Hsinchu",
	"Taipei",
}

// eventCityOrFallback returns the explicit city if set, otherwise attempts to
// extract a known city name from the location string. Returns the raw location
// only if no known city is found (prevents leaking the full address into
// cross-city comparisons).
func eventCityOrFallback(city string, location string) string {
	if strings.TrimSpace(city) != "" {
		return city
	}
	locationLower := strings.ToLower(location)
	for _, c := range knownCities {
		if strings.Contains(locationLower, strings.ToLower(c)) {
			return c
		}
	}
	return location
}

func eventSiteOrFallback(site string, location string) string {
	if strings.TrimSpace(site) != "" {
		return site
	}
	return location
}
