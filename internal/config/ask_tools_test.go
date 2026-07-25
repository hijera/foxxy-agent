package config

import "testing"

func TestAskDisableExtendedToolsDefaultsToOff(t *testing.T) {
	var tools Tools
	if err := tools.Validate(); err != nil {
		t.Fatal(err)
	}
	if tools.AskDisableExtendedTools {
		t.Fatal("ask_disable_extended_tools must default to false")
	}
}

func TestAskDisableExtendedToolsSurvivesJSONRoundTrip(t *testing.T) {
	cfg := &Config{Tools: Tools{
		PermissionMode:          PermModeAsk,
		AskDisableExtendedTools: true,
	}}
	back := JSONDTOToConfig(ConfigToJSONDTO(cfg), Paths{})
	if !back.Tools.AskDisableExtendedTools {
		t.Fatal("ask_disable_extended_tools lost through the JSON config DTO")
	}
}

func TestAskDisableExtendedToolsAppearsAsSettingsCheckbox(t *testing.T) {
	root := UISchemaMap()
	properties := root["properties"].(map[string]interface{})
	tools := properties["tools"].(map[string]interface{})
	toolProperties := tools["properties"].(map[string]interface{})
	field, ok := toolProperties["ask_disable_extended_tools"].(map[string]interface{})
	if !ok {
		t.Fatal("tools.ask_disable_extended_tools missing from UI schema")
	}
	if field["type"] != "boolean" || field["default"] != false {
		t.Fatalf("Ask settings field must be a false-by-default checkbox: %+v", field)
	}
}
