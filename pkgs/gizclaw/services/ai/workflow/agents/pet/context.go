package pet

import (
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func turnInputs(pet apitypes.Pet, petDef apitypes.PetDef) map[string]any {
	return map[string]any{
		"tmp_pet_character_prompt": strings.TrimSpace(petDef.Spec.Character.Prompt),
		"tmp_pet_voice_prompt":     strings.TrimSpace(petDef.Spec.Voice.Prompt),
		"tmp_pet_attribute_prompt": attributePrompt(pet),
	}
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
