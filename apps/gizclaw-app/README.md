# GizClaw App

`apps/gizclaw-app` is the Flutter mobile client for GizClaw. The generated
project targets iOS and Android.

## Current Scope

- Configure a GizClaw server and keep the generated device identity in the
  platform secure store.
- Browse Flowcraft, Doubao, and translation workflows and their workspaces.
- Create and activate workspaces, switch between push-to-talk and realtime
  input, and view or replay workspace history.
- Manage friend invitations and friends, create groups, and open their chatroom
  workspaces.
- List and adopt pets, load their presentation and optional PIXA animation, and
  invoke pet actions.

Workflow, workspace, friend, group, and pet surfaces use the live GizClaw RPCs.
Drift caches the catalog and history data needed for responsive listing and
offline presentation. Prototype fixtures are limited to the demo controller and
widget tests.

## Development

```sh
flutter run
flutter analyze
flutter test
```

For a development server, inject the ignored e2e identity at build time. The
iOS simulator can reach a server on the host through `127.0.0.1`:

```sh
flutter run \
  --dart-define=GIZCLAW_ENDPOINT=127.0.0.1:19820 \
  --dart-define=GIZCLAW_PRIVATE_KEY=<development-private-key>
```

For an Android emulator, use its host alias instead:

```sh
flutter run \
  --dart-define=GIZCLAW_ENDPOINT=10.0.2.2:19820 \
  --dart-define=GIZCLAW_PRIVATE_KEY=<development-private-key>
```

On a physical iOS or Android device, use the development machine's LAN address
and make sure the server listens on that interface.

The Identity screen offers the shared development and production servers as
quick-select endpoints:

- `ap.dev.gizclaw.com:9820`
- `ap.gizclaw.com:9820`

GizClaw servers currently use plain HTTP. An endpoint without an explicit
scheme is therefore interpreted as `http://<host>:<port>`.

After each WebRTC connection is established, the app publishes its current
device information with `server.info.put` and serves `client.info.get` and
`client.identifiers.get` from the same snapshot. The Flutter SDK also dispatches
`client.tool.invoke`; the app returns method-not-found until it registers a
local tool handler.

Do not commit a private key or persist it in Drift. At runtime the app generates
or imports the device key through `flutter_secure_storage`; the endpoint is
stored separately in platform preferences.

Run commands from this directory:

```sh
cd apps/gizclaw-app
```

## Internal Testing

Both mobile publishing workflows are manually dispatched. They use the version
from `pubspec.yaml` and default their platform build number to the GitHub Actions
run number.

### TestFlight Internal Testing

TestFlight publishing runs from `.github/workflows/testflight.yml`. Configure a
protected GitHub Environment named `testflight` before running it.

Create these Apple resources once:

- An explicit App ID for `com.gizclaw.opensource` under team `D782F5CP4S`.
- An App Store Connect app record named `GizClaw OpenSource` using that bundle
  ID.
- An Apple Distribution certificate exported as a password-protected `.p12`.
- An App Store provisioning profile for the bundle ID and distribution
  certificate.
- An App Store Connect team API key with permission to validate and upload
  builds.

Add the following GitHub Environment secrets:

| Secret | Value |
| --- | --- |
| `APP_STORE_CONNECT_KEY_ID` | App Store Connect API key ID. |
| `APP_STORE_CONNECT_ISSUER_ID` | App Store Connect API issuer ID. |
| `APP_STORE_CONNECT_PRIVATE_KEY_BASE64` | Base64-encoded `AuthKey_*.p8`. |
| `IOS_DISTRIBUTION_CERTIFICATE_BASE64` | Base64-encoded distribution `.p12`. |
| `IOS_DISTRIBUTION_CERTIFICATE_PASSWORD` | Password used when exporting the `.p12`. |
| `IOS_PROVISIONING_PROFILE_BASE64` | Base64-encoded App Store `.mobileprovision`. |

On macOS, copy a file as a single-line base64 value with:

```sh
base64 < path/to/file | tr -d '\n' | pbcopy
```

The workflow validates the provisioning profile's team and application
identifier before importing signing material into a temporary keychain. It
marks the export as internal-testing-only, then deletes the keychain, installed
profile, and API private key after the job. Configure an internal TestFlight
group in App Store Connect to distribute processed builds to team members. An
internal-only build cannot later be promoted to external testing or the App
Store.

### Google Play Internal Testing

Google Play publishing runs from
`.github/workflows/google-play-internal.yml`. Configure a protected GitHub
Environment named `google-play-internal` before running it.

Create these Google Play resources once:

- A Google Play Console app named `GizClaw OpenSource` with package name
  `com.gizclaw.opensource`.
- Play App Signing enrollment and a dedicated upload key exported as a Java
  keystore.
- A Google Cloud project with the Google Play Developer API enabled.
- A service account invited in Play Console with permission to publish releases
  to testing tracks.
- An internal tester email list or Google Group in Play Console.

Add the following GitHub Environment secrets:

| Secret | Value |
| --- | --- |
| `GOOGLE_PLAY_SERVICE_ACCOUNT_JSON` | Complete service-account JSON document. |
| `ANDROID_UPLOAD_KEYSTORE_BASE64` | Base64-encoded upload-key keystore. |
| `ANDROID_UPLOAD_KEYSTORE_PASSWORD` | Upload keystore password. |
| `ANDROID_UPLOAD_KEY_ALIAS` | Upload key alias. |
| `ANDROID_UPLOAD_KEY_PASSWORD` | Upload key password. |

The workflow builds a release Android App Bundle signed with the upload key,
verifies its signature, and publishes it with completed status to the Google
Play `internal` track. The keystore is decoded only into the runner's temporary
directory and removed after the job.

## Integration Notes

Mobile presentation will likely need workflow display fields beyond the current
execution contract. Keep those fields in metadata/display-oriented schemas, not
inside workflow driver execution parameters.

Expected future contract work:

- Add display metadata for workflow cards, such as icon, banner image, category,
  featured rank, and short subtitle.
- Add a workflow filter to workspace listing so a workflow detail screen can
  load only its workspaces without client-side filtering.
- Decide whether mobile chat uses Peer OpenAI-compatible chat completions,
  workspace run status/history, or a dedicated chatroom workflow stream for each
  driver.
