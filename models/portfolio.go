package models

type Portfolio struct {
	Player    string         `json:"player" bson:"player"`
	Companies map[string]int `json:"companies" bson:"companies"`
	Funds     float64        `json:"funds" bson:"funds"`
}
