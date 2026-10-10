#!/usr/bin/env python3
"""Cross-repository acceptance against a running GizOS host through Server/Edge.

No package parsing or simulated installation belongs here. The GizOS host owns
installation, saved-data inspection and optional entry/first-frame golden output.
API key is read from GIZCLAW_LUA_APP_API_KEY, never written to receipts.
"""
import argparse
import base64
import hashlib
import http.client
import json
import os
from pathlib import Path
import subprocess
import time
import urllib.parse


def connect(endpoint):
    url = urllib.parse.urlsplit(endpoint)
    if url.scheme not in ('http', 'https') or not url.hostname or url.username or url.fragment:
        raise ValueError('endpoint must be an HTTP(S) origin without credentials')
    connection = http.client.HTTPSConnection if url.scheme == 'https' else http.client.HTTPConnection
    return connection(url.hostname, url.port, timeout=125), url.path.rstrip('/')


def request(endpoint, key, path, body, content_type='application/json', expected=200):
    conn, prefix = connect(endpoint)
    try:
        conn.request('POST', prefix + '/gizclaw/v1/' + path, body=body,
                     headers={'Authorization': 'Bearer ' + key, 'Content-Type': content_type})
        response = conn.getresponse()
        result = json.loads(response.read(65536))
        if response.status != expected:
            raise AssertionError(f'{path.split("?")[0]}: HTTP {response.status}, expected {expected}: {result}')
        return result
    finally:
        conn.close()


def tool(endpoint, key, name, args):
    return request(endpoint, key, 'device/tool/v0/invoke',
                   json.dumps({'tool': name, 'args': args}).encode())['result']


def snapshot(executable):
    # The explicitly supplied GizOS-owned probe must inspect the catalog and
    # user data. Its implementation and golden files live in GizOS.
    value = json.loads(subprocess.check_output([executable], timeout=15))
    if 'app' not in value or 'userdata_sha256' not in value:
        raise ValueError('GizOS probe must return app and userdata_sha256')
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--endpoint', required=True, action='append')
    parser.add_argument('--package', required=True, type=Path)
    parser.add_argument('--url', required=True, action='append', help='Device-accessible HTTP(S) package URL')
    parser.add_argument('--app-id', required=True)
    parser.add_argument('--version', required=True)
    parser.add_argument('--params', default='{}', help='JSON string-to-string launch parameters')
    parser.add_argument('--gizos-probe', required=True, help='GizOS-owned read-only catalog and saved-data probe executable')
    parser.add_argument('--receipt', required=True, type=Path)
    args = parser.parse_args()
    key = os.environ['GIZCLAW_LUA_APP_API_KEY']
    package = args.package.read_bytes()
    if not 65535 < len(package) <= 16 * 1024 * 1024:
        raise ValueError('acceptance package must exceed one RPC frame and be at most 16 MiB')
    sha = hashlib.sha256(package).hexdigest()
    params = json.loads(args.params)
    if not isinstance(params, dict) or not all(isinstance(k, str) and isinstance(v, str) for k, v in params.items()):
        raise ValueError('params must be a string-to-string object')
    receipt = {'archive_bytes': len(package), 'sha256': sha, 'app_id': args.app_id,
               'version': args.version, 'params': params, 'endpoints': []}
    for endpoint in args.endpoint:
        def check_app(result):
            app = result['app']
            assert app['app_id'] == args.app_id and app['version'] == args.version, result
        checks = []
        for url in args.url:
            check_app(tool(endpoint, key, 'lua.app.install', {'url': url, 'sha256': sha}))
            checks.append('url:' + urllib.parse.urlsplit(url).scheme)
        for mime in ['application/zlib', 'application/octet-stream']:
            url = 'data:' + mime + ';base64,' + base64.b64encode(package).decode('ascii')
            if len(url) > 262144:
                raise ValueError('data URL acceptance fixture must fit 256 KiB; use Binary for larger packages')
            check_app(tool(endpoint, key, 'lua.app.install', {'url': url, 'sha256': sha}))
            checks.append('data:' + mime)
        before = snapshot(args.gizos_probe)
        path = 'device/lua-app/install?' + urllib.parse.urlencode({'content_length': len(package), 'sha256': sha})
        for label, body, route in [
            ('truncated', package[:-1], path),
            ('overlong', package + b'\0', path),
            ('bad_sha', package, path.replace(sha, '0' * 64)),
        ]:
            result = request(endpoint, key, route, body, 'application/octet-stream', 400)
            assert result['error']['code'] == 'DEVICE_REJECTED', result
            assert snapshot(args.gizos_probe) == before, label + ' changed application or saved data'
            checks.append(label)
        # Cancel one still-incomplete HTTP body. It must never receive RPC EOS.
        conn, prefix = connect(endpoint)
        conn.putrequest('POST', prefix + '/gizclaw/v1/' + path)
        conn.putheader('Authorization', 'Bearer ' + key)
        conn.putheader('Content-Type', 'application/octet-stream')
        conn.putheader('Content-Length', str(len(package)))
        conn.endheaders()
        conn.send(package[:32768])
        conn.close()
        time.sleep(0.2)
        assert snapshot(args.gizos_probe) == before, 'cancellation changed application or saved data'
        checks.append('cancel')
        check_app(request(endpoint, key, path, package, 'application/octet-stream'))
        checks.append('binary')
        catalog = tool(endpoint, key, 'lua.app.list', {})['apps']
        assert any(a['app_id'] == args.app_id and a['version'] == args.version for a in catalog)
        run = tool(endpoint, key, 'lua.app.run', {'app_id': args.app_id, 'params': params})
        observed = snapshot(args.gizos_probe)
        for _ in range(50):
            if observed.get('last_run') == {'app_id': args.app_id, 'params': params}:
                break
            time.sleep(0.1)
            observed = snapshot(args.gizos_probe)
        else:
            raise AssertionError('GizOS did not observe the exact launch parameters')
        receipt['endpoints'].append({'endpoint': endpoint, 'checks': checks, 'catalog': catalog, 'run': run,
                                     'gizos': observed})
    args.receipt.write_text(json.dumps(receipt, indent=2, ensure_ascii=False) + '\n')
    print(json.dumps({'receipt': str(args.receipt), 'sha256': sha, 'archive_bytes': len(package)}))


if __name__ == '__main__':
    main()
