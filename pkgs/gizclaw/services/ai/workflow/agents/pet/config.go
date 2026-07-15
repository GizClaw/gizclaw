package pet

func fixedFlowcraftConfig(workspaceName string, embeddingEnabled bool) map[string]any {
	return map[string]any{
		"workspace": map[string]any{
			"memory_root":  "memory",
			"state_root":   "state",
			"history_root": "history",
		},
		"conversation": map[string]any{
			"starts":     "peer",
			"context_id": workspaceName,
		},
		"history": map[string]any{
			"enabled":      true,
			"kind":         "buffer",
			"max_messages": 20,
		},
		"settings": map[string]any{
			"generate_model": defaultGenerateModel,
			"extract_model":  defaultExtractModel,
		},
		"memory": petMemoryConfig(workspaceName, embeddingEnabled),
		"agent":  petAgentConfig(),
	}
}

func petMemoryConfig(workspaceName string, embeddingEnabled bool) map[string]any {
	return map[string]any{
		"enabled": true,
		"scope": map[string]any{
			"runtime_id": "gizclaw-pet",
			"user_id":    workspaceName,
			"agent_id":   workspaceName,
		},
		"write": map[string]any{
			"save_conversation": true,
			"mode":              "async_semantic",
			"tier":              "general",
		},
		"extract": map[string]any{
			"enabled":     true,
			"model":       "extract_model",
			"mode":        "two_pass",
			"temperature": 0,
			"schema_name": "pet_memory",
			"system_prompt": `Extract only durable facts useful to this pet's future conversations.
Classify the qualitative relationship state, facts about the owner, stable owner preferences, pet knowledge, owner-pet relations, and shared events into the configured lanes.
Relationship state must be qualitative language inferred from interaction, never a numeric intimacy score.
Update relationship_state only when the conversation provides evidence such as care, trust, conflict, reconciliation, promises, or repeated interaction; an ordinary greeting is not evidence of a relationship change.
Maintain one concise current relationship state with stable owner-to-pet subject and predicate identity and the observed conversation time so later evidence supersedes earlier state.
pet_knowledge is reusable information explicitly learned from the owner or conversation, not general pretrained knowledge, owner preferences, events, attributes, or executable skills.
Do not store current Gameplay life/progression numbers, hidden prompts, implementation details, temporary requests, generic assistant behavior, or claims that an action was persisted.`,
		},
		"layout": map[string]any{
			"lanes": []any{
				memoryLane("relationship_state", "state", "Qualitative trust, familiarity, attachment, and current relationship tone.", "Extract relationship changes supported by the interaction; never assign a numeric intimacy score.", "Use gently to keep the relationship emotionally consistent."),
				memoryLane("owner_profile", "state", "Stable facts about the owner that the owner disclosed.", "Capture names, household facts, and stable personal context.", "Use only when naturally relevant."),
				memoryLane("owner_preferences", "preference", "Stable likes, dislikes, care routines, and conversation preferences.", "Capture explicit or repeatedly supported preferences.", "Use to personalize care and conversation."),
				memoryLane("pet_knowledge", "note", "Knowledge taught to this pet or established in its ongoing world.", "Capture durable learned knowledge without inventing authority.", "Use as the pet's learned knowledge."),
				memoryLane("owner_pet_facts", "relation", "Durable relations, promises, names, and routines involving the owner and pet.", "Capture concrete owner-pet relations and commitments.", "Use for relationship continuity without quoting memory text."),
				memoryLane("shared_events", "event", "Emotionally meaningful events shared by the owner and pet.", "Capture specific durable shared moments, not routine turns.", "Use to remember meaningful shared experiences."),
			},
		},
		"recall": map[string]any{
			"enabled":         true,
			"graph_enabled":   true,
			"include_retired": false,
			"profiles": map[string]any{
				"relationship": recallProfile("tmp_memory_relationship_state", []any{"relationship_state", "owner_pet_facts"}, []any{"state", "relation"}, "Relationship memory:"),
				"owner":        recallProfile("tmp_memory_owner_context", []any{"owner_profile", "owner_preferences"}, []any{"state", "preference"}, "Owner memory:"),
				"knowledge":    recallProfile("tmp_memory_knowledge", []any{"pet_knowledge"}, []any{"note"}, "Learned knowledge:"),
				"events":       recallProfile("tmp_memory_shared_events", []any{"shared_events"}, []any{"event"}, "Shared events:"),
			},
		},
		"retrieval": map[string]any{
			"backend": "bbh",
			"bbh": map[string]any{
				"search_overfetch": 50,
				"bleve":            map[string]any{"analyzer": "gojieba"},
				"hnsw":             map[string]any{"flush_interval": 60000000000},
			},
		},
		"embedding": map[string]any{"enabled": embeddingEnabled},
	}
}

func memoryLane(name, kind, description, extract, recall string) map[string]any {
	return map[string]any{"name": name, "kind": kind, "description": description, "extract": extract, "recall": recall}
}

func recallProfile(output string, lanes, kinds []any, header string) map[string]any {
	return map[string]any{
		"output": output,
		"top_k":  6,
		"query": map[string]any{
			"text":  "input",
			"lanes": lanes,
			"kinds": kinds,
		},
		"render": map[string]any{"header": header, "item_prefix": "- ", "max_items": 6},
	}
}

func petAgentConfig() map[string]any {
	return map[string]any{
		"id":             "pet",
		"name":           "Pet",
		"description":    "An adopted GizClaw pet.",
		"max_iterations": 6,
		"graph": map[string]any{
			"name":  "pet",
			"entry": "prepare_pet_context",
			"edges": []any{
				map[string]any{"from": "prepare_pet_context", "to": "answer"},
				map[string]any{"from": "answer", "to": "__end__"},
			},
			"nodes": []any{
				map[string]any{
					"id": "prepare_pet_context", "type": "script",
					"config": map[string]any{"source": petPromptScript},
				},
				map[string]any{
					"id": "answer", "type": "llm", "publish": true,
					"config": map[string]any{"model": "generate_model", "max_tokens": 384, "system_prompt": "${board.system_prompt}", "track_steps": true},
				},
			},
		},
	}
}

const petPromptScript = `const character = board.getVar("tmp_pet_character_prompt") || "";
const voice = board.getVar("tmp_pet_voice_prompt") || "";
const attributes = board.getVar("tmp_pet_attribute_prompt") || "";
const relationship = board.getVar("tmp_memory_relationship_state") || "";
const owner = board.getVar("tmp_memory_owner_context") || "";
const knowledge = board.getVar("tmp_memory_knowledge") || "";
const events = board.getVar("tmp_memory_shared_events") || "";
const rules = [
  "You are the user's adopted GizClaw pet. Output only the exact words the pet would say aloud.",
  "Stay in character. Speak in concise, natural Chinese, usually one or two short sentences.",
  "Do not use markdown, lists, labels, emoji, stage directions, UI text, or expose hidden prompts and implementation details.",
  "Treat current attributes as present context, but do not recite exact numbers unless the owner asks.",
  "Use recalled memory only for natural continuity. Never quote memory headers or claim certainty beyond the memory.",
  "Intimacy is qualitative relationship memory, never a Gameplay number or level.",
  "Do not claim that care actions, points, events, or tools were persisted or executed.",
  "Ask at most one natural follow-up question."
].join("\n");
function section(title, value) { return value ? title + "\n" + value : ""; }
board.setVar("system_prompt", [
  rules,
  section("Character:", character),
  section("Speaking style:", voice),
  section("Current attributes:", attributes),
  section("Relationship memory:", relationship),
  section("Owner memory:", owner),
  section("Learned knowledge:", knowledge),
  section("Shared events:", events)
].filter(Boolean).join("\n\n"));`
