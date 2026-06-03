package ticketing

type AdminOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type AdminHROptions struct {
	Sites []AdminOption `json:"sites"`
}
