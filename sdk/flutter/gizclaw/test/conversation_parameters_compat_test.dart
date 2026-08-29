// ignore_for_file: deprecated_member_use_from_same_package

import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

void main() {
  test('keeps Flowcraft conversation parameter source aliases', () {
    final FlowcraftConversationParameters
    legacy = FlowcraftConversationParameters(
      initiative: FlowcraftConversationParametersInitiative
          .FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_AGENT,
      agentInitiativePolicy: FlowcraftConversationParametersAgentInitiativePolicy
          .FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ON_RELOAD,
    );
    final ConversationParameters current = legacy;

    expect(
      current.initiative,
      FlowcraftConversationParametersInitiative
          .FLOWCRAFT_CONVERSATION_PARAMETERS_INITIATIVE_AGENT,
    );
    expect(
      current.agentInitiativePolicy,
      FlowcraftConversationParametersAgentInitiativePolicy
          .FLOWCRAFT_CONVERSATION_PARAMETERS_AGENT_INITIATIVE_POLICY_ON_RELOAD,
    );
  });
}
