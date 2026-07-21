package pet

import (
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func turnInputs(pet apitypes.Pet, petDef apitypes.PetDef, parameters apitypes.PetWorkspaceParameters) map[string]any {
	return map[string]any{
		"tmp_pet_character_prompt": joinPrompts(petDef.Spec.Character.Prompt, personaPrompt(parameters)),
		"tmp_pet_voice_prompt":     joinPrompts(petDef.Spec.Voice.Prompt, voicePrompt(parameters)),
		"tmp_pet_attribute_prompt": attributePrompt(pet),
	}
}

func personaPrompt(parameters apitypes.PetWorkspaceParameters) string {
	if parameters.Persona == nil || parameters.Persona.Prompt == nil {
		return ""
	}
	return *parameters.Persona.Prompt
}

func voicePrompt(parameters apitypes.PetWorkspaceParameters) string {
	if parameters.Voice.Prompt == nil {
		return ""
	}
	return *parameters.Voice.Prompt
}

func joinPrompts(prompts ...string) string {
	parts := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		if prompt = strings.TrimSpace(prompt); prompt != "" {
			parts = append(parts, prompt)
		}
	}
	return strings.Join(parts, "\n\n")
}

func attributePrompt(pet apitypes.Pet) string {
	sections := make([]string, 0, 4)
	if name := strings.TrimSpace(pet.DisplayName); name != "" {
		sections = append(sections, "当前名字："+name)
	}
	sections = append(sections, fmt.Sprintf(
		"当前生活属性：life=%.2f，health=%.2f，satiety=%.2f，hygiene=%.2f，mood=%.2f，energy=%.2f",
		pet.Stats.Life, pet.Stats.Health, pet.Stats.Satiety, pet.Stats.Hygiene, pet.Stats.Mood, pet.Stats.Energy,
	))
	sections = append(sections, fmt.Sprintf("当前成长属性：experience=%d，level=%d", pet.Progression.Experience, pet.Progression.Level))
	sections = append(sections, "当前生命周期："+string(pet.Lifecycle))
	return strings.Join(sections, "\n")
}
