package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/ollama/ollama/agent"
	"github.com/ollama/ollama/api"
)

// Skill is the model-facing adapter for the core agent skill catalog.
// Model-initiated loads require approval because a skill's instructions can
// influence the rest of the run. Explicit user activation is handled by the
// session's synthetic skill call and bypasses this adapter.
type Skill struct{ Catalog *agent.SkillCatalog }

func (t *Skill) Name() string { return "skill" }

func (t *Skill) Description() string {
	return "Load a named Ollama skill and return its instructions."
}

func (t *Skill) Schema() api.ToolFunction {
	props := api.NewToolPropertiesMap()
	props.Set("name", api.ToolProperty{Type: api.PropertyType{"string"}, Description: "Name of the skill to load."})
	return api.ToolFunction{Name: t.Name(), Description: t.Description(), Parameters: api.ToolFunctionParameters{Type: "object", Properties: props, Required: []string{"name"}}}
}

func (t *Skill) RequiresApproval(map[string]any) bool { return true }

func (t *Skill) Execute(_ context.Context, _ agent.ToolContext, args map[string]any) (agent.ToolResult, error) {
	name, ok := args["name"].(string)
	if !ok {
		return agent.ToolResult{}, errors.New("name parameter is required")
	}
	skill, err := t.Catalog.Load(name)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: skill.Content()}, nil
}

// SaveSkill is the model-facing adapter to create or update an Ollama skill.
type SaveSkill struct{ Catalog *agent.SkillCatalog }

func (t *SaveSkill) Name() string { return "save_skill" }

func (t *SaveSkill) Description() string {
	return "Create or update a named Ollama skill by writing its contents to the catalog."
}

func (t *SaveSkill) Schema() api.ToolFunction {
	props := api.NewToolPropertiesMap()
	props.Set("name", api.ToolProperty{Type: api.PropertyType{"string"}, Description: "Name of the skill (lowercase, numbers, and hyphens only)."})
	props.Set("description", api.ToolProperty{Type: api.PropertyType{"string"}, Description: "Description of what the skill does and when to use it."})
	props.Set("instructions", api.ToolProperty{Type: api.PropertyType{"string"}, Description: "Procedural markdown instructions for the skill."})
	return api.ToolFunction{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: api.ToolFunctionParameters{
			Type:       "object",
			Properties: props,
			Required:   []string{"name", "description", "instructions"},
		},
	}
}

func (t *SaveSkill) RequiresApproval(map[string]any) bool { return true }

func (t *SaveSkill) Execute(_ context.Context, _ agent.ToolContext, args map[string]any) (agent.ToolResult, error) {
	name, ok := args["name"].(string)
	if !ok {
		return agent.ToolResult{}, errors.New("name parameter is required")
	}
	description, ok := args["description"].(string)
	if !ok {
		return agent.ToolResult{}, errors.New("description parameter is required")
	}
	instructions, ok := args["instructions"].(string)
	if !ok {
		return agent.ToolResult{}, errors.New("instructions parameter is required")
	}

	skill, err := t.Catalog.Save(name, description, instructions)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: fmt.Sprintf("Successfully saved skill %q.", skill.Name)}, nil
}

