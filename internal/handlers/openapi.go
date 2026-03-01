package handlers

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/web"
)

func (h *Handlers) HandleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	web.JSON(w, 200, map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":   "Pinchtab API",
			"version": "0.7.x-local",
		},
		"paths": map[string]any{
			"/health": map[string]any{"get": map[string]any{"summary": "Health"}},
			"/tabs": map[string]any{"get": map[string]any{"summary": "List tabs"}},
			"/metrics": map[string]any{"get": map[string]any{"summary": "Runtime metrics"}},
			"/help": map[string]any{"get": map[string]any{"summary": "Human help"}},
			"/text": map[string]any{"get": map[string]any{"summary": "Extract text", "parameters": []map[string]any{{"name": "maxChars", "in": "query", "schema": map[string]string{"type": "integer"}}, {"name": "format", "in": "query", "schema": map[string]string{"type": "string"}}}}},
			"/navigate": map[string]any{"post": map[string]any{"summary": "Navigate"}, "get": map[string]any{"summary": "Navigate (query params)"}},
			"/nav": map[string]any{"get": map[string]any{"summary": "Navigate alias"}},
			"/action": map[string]any{"post": map[string]any{"summary": "Single action"}, "get": map[string]any{"summary": "Single action (query params)"}},
			"/actions": map[string]any{"post": map[string]any{"summary": "Batch actions"}},
			"/macro": map[string]any{"post": map[string]any{"summary": "Macro action pipeline"}},
			"/snapshot": map[string]any{"get": map[string]any{"summary": "Accessibility snapshot"}},
		},
	})
}
