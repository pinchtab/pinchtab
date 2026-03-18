package config

import (
	"encoding/json"
	"fmt"
)

// PatchConfigJSON merges a JSON object into FileConfig.
func PatchConfigJSON(fc *FileConfig, jsonPatch string) error {
	current, err := json.Marshal(fc)
	if err != nil {
		return fmt.Errorf("failed to serialize current config: %w", err)
	}

	var currentMap map[string]interface{}
	if err := json.Unmarshal(current, &currentMap); err != nil {
		return fmt.Errorf("failed to parse current config: %w", err)
	}

	var patchMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPatch), &patchMap); err != nil {
		return fmt.Errorf("invalid JSON patch: %w", err)
	}

	merged := deepMerge(currentMap, patchMap)

	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("failed to serialize merged config: %w", err)
	}

	if err := json.Unmarshal(mergedJSON, fc); err != nil {
		return fmt.Errorf("failed to parse merged config: %w", err)
	}

	return nil
}

func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range dst {
		result[k] = v
	}
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := result[k].(map[string]interface{}); ok {
				result[k] = deepMerge(dstMap, srcMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}
