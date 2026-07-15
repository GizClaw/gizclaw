package pet

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func turnInputs(pet apitypes.Pet, petDef apitypes.PetDef, parameters apitypes.PetWorkspaceParameters) map[string]any {
	return map[string]any{
		"tmp_pet_character_prompt": joinPrompts(petDef.Spec.Character.Prompt, personaPrompt(parameters)),
		"tmp_pet_voice_prompt":     joinPrompts(petDef.Spec.Voice.Prompt, voicePrompt(parameters)),
		"tmp_pet_attribute_prompt": attributePrompt(pet, petDef),
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

func attributePrompt(pet apitypes.Pet, petDef apitypes.PetDef) string {
	sections := make([]string, 0, 2)
	if name := strings.TrimSpace(pet.DisplayName); name != "" {
		sections = append(sections, "当前名字："+name)
	}
	if values := definedAttributeValues(petDef.Spec.Attr.Life, pet.Life); len(values) > 0 {
		sections = append(sections, "当前生活属性："+strings.Join(values, "，"))
	}
	if values := definedAttributeValues(petDef.Spec.Attr.Progression, pet.Progression); len(values) > 0 {
		sections = append(sections, "当前成长属性："+strings.Join(values, "，"))
	}
	return strings.Join(sections, "\n")
}

func definedAttributeValues(definitions apitypes.PetAttrGroupSpec, values map[string]int64) []string {
	keys := make([]string, 0, len(definitions))
	for key := range definitions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, fmt.Sprintf("%s=%d", key, values[key]))
	}
	return out
}
