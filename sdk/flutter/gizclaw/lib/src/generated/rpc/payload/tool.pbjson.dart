// This is a generated file - do not edit.
//
// Generated from payload/tool.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports
// ignore_for_file: unused_import

import 'dart:convert' as $convert;
import 'dart:core' as $core;
import 'dart:typed_data' as $typed_data;

@$core.Deprecated('Use clientToolDescriptor instead')
const ClientTool$json = {
  '1': 'ClientTool',
  '2': [
    {'1': 'CLIENT_TOOL_UNSPECIFIED', '2': 0},
    {'1': 'CLIENT_TOOL_INFO_GET', '2': 1, '3': {}},
    {'1': 'CLIENT_TOOL_IDENTIFIERS_GET', '2': 2, '3': {}},
    {'1': 'CLIENT_TOOL_DEVICE_STATUS_GET', '2': 3, '3': {}},
    {'1': 'CLIENT_TOOL_DEVICE_REBOOT', '2': 4, '3': {}},
    {'1': 'CLIENT_TOOL_DEVICE_FACTORY_RESET', '2': 5, '3': {}},
    {'1': 'CLIENT_TOOL_DEVICE_FIND', '2': 6, '3': {}},
    {'1': 'CLIENT_TOOL_SOUND_PLAY', '2': 7, '3': {}},
    {'1': 'CLIENT_TOOL_WIFI_SCAN', '2': 8, '3': {}},
    {'1': 'CLIENT_TOOL_WIFI_CONNECT', '2': 9, '3': {}},
    {'1': 'CLIENT_TOOL_WIFI_SAVED_LIST', '2': 10, '3': {}},
    {'1': 'CLIENT_TOOL_WIFI_SAVED_FORGET', '2': 11, '3': {}},
    {'1': 'CLIENT_TOOL_FIRMWARE_UPDATE', '2': 12, '3': {}},
    {'1': 'CLIENT_TOOL_AUDIOPLAYER_GET', '2': 13, '3': {}},
    {'1': 'CLIENT_TOOL_AUDIOPLAYER_PLAY', '2': 14, '3': {}},
    {'1': 'CLIENT_TOOL_AUDIOPLAYER_STOP', '2': 15, '3': {}},
    {'1': 'CLIENT_TOOL_AUDIOPLAYER_MODE_SET', '2': 16, '3': {}},
    {'1': 'CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET', '2': 17, '3': {}},
    {'1': 'CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_SET', '2': 18, '3': {}},
    {'1': 'CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_APPEND', '2': 19, '3': {}},
    {'1': 'CLIENT_TOOL_RUN_WORKSPACE_SET', '2': 20, '3': {}},
    {'1': 'CLIENT_TOOL_SOCIAL_PING', '2': 21, '3': {}},
  ],
};

/// Descriptor for `ClientTool`. Decode as a `google.protobuf.EnumDescriptorProto`.
final $typed_data.Uint8List clientToolDescriptor = $convert.base64Decode(
    'CgpDbGllbnRUb29sEhsKF0NMSUVOVF9UT09MX1VOU1BFQ0lGSUVEEAASVQoUQ0xJRU5UX1RPT0'
    'xfSU5GT19HRVQQARo7yvMYNwoIaW5mby5nZXQSFENsaWVudEdldEluZm9SZXF1ZXN0GhVDbGll'
    'bnRHZXRJbmZvUmVzcG9uc2UScQobQ0xJRU5UX1RPT0xfSURFTlRJRklFUlNfR0VUEAIaUMrzGE'
    'wKD2lkZW50aWZpZXJzLmdldBIbQ2xpZW50R2V0SWRlbnRpZmllcnNSZXF1ZXN0GhxDbGllbnRH'
    'ZXRJZGVudGlmaWVyc1Jlc3BvbnNlEncKHUNMSUVOVF9UT09MX0RFVklDRV9TVEFUVVNfR0VUEA'
    'MaVMrzGFAKEWRldmljZS5zdGF0dXMuZ2V0EhxDbGllbnREZXZpY2VTdGF0dXNHZXRSZXF1ZXN0'
    'Gh1DbGllbnREZXZpY2VTdGF0dXNHZXRSZXNwb25zZRJpChlDTElFTlRfVE9PTF9ERVZJQ0VfUk'
    'VCT09UEAQaSsrzGEYKDWRldmljZS5yZWJvb3QSGUNsaWVudERldmljZVJlYm9vdFJlcXVlc3Qa'
    'GkNsaWVudERldmljZVJlYm9vdFJlc3BvbnNlEoMBCiBDTElFTlRfVE9PTF9ERVZJQ0VfRkFDVE'
    '9SWV9SRVNFVBAFGl3K8xhZChRkZXZpY2UuZmFjdG9yeV9yZXNldBIfQ2xpZW50RGV2aWNlRmFj'
    'dG9yeVJlc2V0UmVxdWVzdBogQ2xpZW50RGV2aWNlRmFjdG9yeVJlc2V0UmVzcG9uc2USYQoXQ0'
    'xJRU5UX1RPT0xfREVWSUNFX0ZJTkQQBhpEyvMYQAoLZGV2aWNlLmZpbmQSF0NsaWVudERldmlj'
    'ZUZpbmRSZXF1ZXN0GhhDbGllbnREZXZpY2VGaW5kUmVzcG9uc2USaQoWQ0xJRU5UX1RPT0xfU0'
    '9VTkRfUExBWRAHGk3K8xhJCgpzb3VuZC5wbGF5EhxDbGllbnREZXZpY2VTb3VuZFBsYXlSZXF1'
    'ZXN0Gh1DbGllbnREZXZpY2VTb3VuZFBsYXlSZXNwb25zZRJZChVDTElFTlRfVE9PTF9XSUZJX1'
    'NDQU4QCBo+yvMYOgoJd2lmaS5zY2FuEhVDbGllbnRXaWZpU2NhblJlcXVlc3QaFkNsaWVudFdp'
    'ZmlTY2FuUmVzcG9uc2USZQoYQ0xJRU5UX1RPT0xfV0lGSV9DT05ORUNUEAkaR8rzGEMKDHdpZm'
    'kuY29ubmVjdBIYQ2xpZW50V2lmaUNvbm5lY3RSZXF1ZXN0GhlDbGllbnRXaWZpQ29ubmVjdFJl'
    'c3BvbnNlEm8KG0NMSUVOVF9UT09MX1dJRklfU0FWRURfTElTVBAKGk7K8xhKCg93aWZpLnNhdm'
    'VkLmxpc3QSGkNsaWVudFdpZmlTYXZlZExpc3RSZXF1ZXN0GhtDbGllbnRXaWZpU2F2ZWRMaXN0'
    'UmVzcG9uc2USdwodQ0xJRU5UX1RPT0xfV0lGSV9TQVZFRF9GT1JHRVQQCxpUyvMYUAoRd2lmaS'
    '5zYXZlZC5mb3JnZXQSHENsaWVudFdpZmlTYXZlZEZvcmdldFJlcXVlc3QaHUNsaWVudFdpZmlT'
    'YXZlZEZvcmdldFJlc3BvbnNlEnEKG0NMSUVOVF9UT09MX0ZJUk1XQVJFX1VQREFURRAMGlDK8x'
    'hMCg9maXJtd2FyZS51cGRhdGUSG0NsaWVudEZpcm13YXJlVXBkYXRlUmVxdWVzdBocQ2xpZW50'
    'RmlybXdhcmVVcGRhdGVSZXNwb25zZRJ9ChtDTElFTlRfVE9PTF9BVURJT1BMQVlFUl9HRVQQDR'
    'pcyvMYWAoPYXVkaW9wbGF5ZXIuZ2V0EiFDbGllbnREZXZpY2VBdWRpb1BsYXllckdldFJlcXVl'
    'c3QaIkNsaWVudERldmljZUF1ZGlvUGxheWVyR2V0UmVzcG9uc2USgQEKHENMSUVOVF9UT09MX0'
    'FVRElPUExBWUVSX1BMQVkQDhpfyvMYWwoQYXVkaW9wbGF5ZXIucGxheRIiQ2xpZW50RGV2aWNl'
    'QXVkaW9QbGF5ZXJQbGF5UmVxdWVzdBojQ2xpZW50RGV2aWNlQXVkaW9QbGF5ZXJQbGF5UmVzcG'
    '9uc2USgQEKHENMSUVOVF9UT09MX0FVRElPUExBWUVSX1NUT1AQDxpfyvMYWwoQYXVkaW9wbGF5'
    'ZXIuc3RvcBIiQ2xpZW50RGV2aWNlQXVkaW9QbGF5ZXJTdG9wUmVxdWVzdBojQ2xpZW50RGV2aW'
    'NlQXVkaW9QbGF5ZXJTdG9wUmVzcG9uc2USjwEKIENMSUVOVF9UT09MX0FVRElPUExBWUVSX01P'
    'REVfU0VUEBAaacrzGGUKFGF1ZGlvcGxheWVyLm1vZGUuc2V0EiVDbGllbnREZXZpY2VBdWRpb1'
    'BsYXllck1vZGVTZXRSZXF1ZXN0GiZDbGllbnREZXZpY2VBdWRpb1BsYXllck1vZGVTZXRSZXNw'
    'b25zZRKfAQokQ0xJRU5UX1RPT0xfQVVESU9QTEFZRVJfUExBWUxJU1RfR0VUEBEadcrzGHEKGG'
    'F1ZGlvcGxheWVyLnBsYXlsaXN0LmdldBIpQ2xpZW50RGV2aWNlQXVkaW9QbGF5ZXJQbGF5bGlz'
    'dEdldFJlcXVlc3QaKkNsaWVudERldmljZUF1ZGlvUGxheWVyUGxheWxpc3RHZXRSZXNwb25zZR'
    'KfAQokQ0xJRU5UX1RPT0xfQVVESU9QTEFZRVJfUExBWUxJU1RfU0VUEBIadcrzGHEKGGF1ZGlv'
    'cGxheWVyLnBsYXlsaXN0LnNldBIpQ2xpZW50RGV2aWNlQXVkaW9QbGF5ZXJQbGF5bGlzdFNldF'
    'JlcXVlc3QaKkNsaWVudERldmljZUF1ZGlvUGxheWVyUGxheWxpc3RTZXRSZXNwb25zZRKrAQon'
    'Q0xJRU5UX1RPT0xfQVVESU9QTEFZRVJfUExBWUxJU1RfQVBQRU5EEBMafsrzGHoKG2F1ZGlvcG'
    'xheWVyLnBsYXlsaXN0LmFwcGVuZBIsQ2xpZW50RGV2aWNlQXVkaW9QbGF5ZXJQbGF5bGlzdEFw'
    'cGVuZFJlcXVlc3QaLUNsaWVudERldmljZUF1ZGlvUGxheWVyUGxheWxpc3RBcHBlbmRSZXNwb2'
    '5zZRJ3Ch1DTElFTlRfVE9PTF9SVU5fV09SS1NQQUNFX1NFVBAUGlTK8xhQChFydW4ud29ya3Nw'
    'YWNlLnNldBIcQ2xpZW50UnVuV29ya3NwYWNlU2V0UmVxdWVzdBodQ2xpZW50UnVuV29ya3NwYW'
    'NlU2V0UmVzcG9uc2USYQoXQ0xJRU5UX1RPT0xfU09DSUFMX1BJTkcQFRpEyvMYQAoLc29jaWFs'
    'LnBpbmcSF0NsaWVudFNvY2lhbFBpbmdSZXF1ZXN0GhhDbGllbnRTb2NpYWxQaW5nUmVzcG9uc2'
    'U=');

@$core.Deprecated('Use clientToolOptionsDescriptor instead')
const ClientToolOptions$json = {
  '1': 'ClientToolOptions',
  '2': [
    {'1': 'name', '3': 1, '4': 1, '5': 9, '10': 'name'},
    {'1': 'request', '3': 2, '4': 1, '5': 9, '10': 'request'},
    {'1': 'response', '3': 3, '4': 1, '5': 9, '10': 'response'},
  ],
};

/// Descriptor for `ClientToolOptions`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientToolOptionsDescriptor = $convert.base64Decode(
    'ChFDbGllbnRUb29sT3B0aW9ucxISCgRuYW1lGAEgASgJUgRuYW1lEhgKB3JlcXVlc3QYAiABKA'
    'lSB3JlcXVlc3QSGgoIcmVzcG9uc2UYAyABKAlSCHJlc3BvbnNl');

@$core.Deprecated('Use clientToolV0InvokeRequestDescriptor instead')
const ClientToolV0InvokeRequest$json = {
  '1': 'ClientToolV0InvokeRequest',
  '2': [
    {
      '1': 'tool',
      '3': 1,
      '4': 1,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ClientTool',
      '10': 'tool'
    },
    {
      '1': 'payload',
      '3': 2,
      '4': 1,
      '5': 12,
      '9': 0,
      '10': 'payload',
      '17': true
    },
  ],
  '8': [
    {'1': '_payload'},
  ],
};

/// Descriptor for `ClientToolV0InvokeRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientToolV0InvokeRequestDescriptor = $convert.base64Decode(
    'ChlDbGllbnRUb29sVjBJbnZva2VSZXF1ZXN0Ei4KBHRvb2wYASABKA4yGi5naXpjbGF3LnJwYy'
    '52MS5DbGllbnRUb29sUgR0b29sEh0KB3BheWxvYWQYAiABKAxIAFIHcGF5bG9hZIgBAUIKCghf'
    'cGF5bG9hZA==');

@$core.Deprecated('Use clientToolV0InvokeResponseDescriptor instead')
const ClientToolV0InvokeResponse$json = {
  '1': 'ClientToolV0InvokeResponse',
  '2': [
    {
      '1': 'payload',
      '3': 1,
      '4': 1,
      '5': 12,
      '9': 0,
      '10': 'payload',
      '17': true
    },
  ],
  '8': [
    {'1': '_payload'},
  ],
};

/// Descriptor for `ClientToolV0InvokeResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientToolV0InvokeResponseDescriptor =
    $convert.base64Decode(
        'ChpDbGllbnRUb29sVjBJbnZva2VSZXNwb25zZRIdCgdwYXlsb2FkGAEgASgMSABSB3BheWxvYW'
        'SIAQFCCgoIX3BheWxvYWQ=');

@$core.Deprecated('Use clientToolV0ListRequestDescriptor instead')
const ClientToolV0ListRequest$json = {
  '1': 'ClientToolV0ListRequest',
};

/// Descriptor for `ClientToolV0ListRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientToolV0ListRequestDescriptor =
    $convert.base64Decode('ChdDbGllbnRUb29sVjBMaXN0UmVxdWVzdA==');

@$core.Deprecated('Use clientToolV0ListResponseDescriptor instead')
const ClientToolV0ListResponse$json = {
  '1': 'ClientToolV0ListResponse',
  '2': [
    {
      '1': 'tools',
      '3': 1,
      '4': 3,
      '5': 14,
      '6': '.gizclaw.rpc.v1.ClientTool',
      '10': 'tools'
    },
  ],
};

/// Descriptor for `ClientToolV0ListResponse`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientToolV0ListResponseDescriptor =
    $convert.base64Decode(
        'ChhDbGllbnRUb29sVjBMaXN0UmVzcG9uc2USMAoFdG9vbHMYASADKA4yGi5naXpjbGF3LnJwYy'
        '52MS5DbGllbnRUb29sUgV0b29scw==');
