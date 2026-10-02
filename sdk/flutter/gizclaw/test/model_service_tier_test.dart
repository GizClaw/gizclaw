import 'package:gizclaw/gizclaw.dart';
import 'package:test/test.dart';

void main() {
  test('Volc model service tier survives protobuf with optional presence', () {
    final response = ModelGetResponse(
      value: Model(
        name: 'doubao',
        kind: ModelKind.MODEL_KIND_LLM,
        providerKind: ModelProviderKind.MODEL_PROVIDER_KIND_VOLC_TENANT,
        volcTenant: VolcTenantModelProviderData(
          apiMode: 'chat_completions',
          upstreamModel: 'doubao-test',
          serviceTier: 'fast',
        ),
      ),
    );
    final decoded = ModelGetResponse.fromBuffer(response.writeToBuffer());
    expect(decoded.value.volcTenant.hasServiceTier(), isTrue);
    expect(decoded.value.volcTenant.serviceTier, 'fast');
    response.value.volcTenant.clearServiceTier();
    final omitted = ModelGetResponse.fromBuffer(response.writeToBuffer());
    expect(omitted.value.volcTenant.hasServiceTier(), isFalse);
  });
}
