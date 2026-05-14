package ticketing

import "strings"

func eventCityOrFallback(city string, location string) string {
	if strings.TrimSpace(city) != "" {
		return city
	}
	return location
}

func eventSiteOrFallback(site string, location string) string {
	if strings.TrimSpace(site) != "" {
		return site
	}
	return location
}
