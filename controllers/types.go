package controllers

type ProxmoxNodeResponse struct {
	Level string `json:"level,omitempty"`
	Node  string `json:"node"`
	Type  string `json:"type"`
	Id    string `json:"id"`
}
