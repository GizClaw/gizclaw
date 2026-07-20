# Wails App

The entire window of GizClaw Desktop is a collection of Pods: there is no web-style title, description area, search bar,
Sidebar or page navigation. Pod uses a small card grid of the same size as the added entry; when there is no Pod, the screen
Keep only the centered add card. After clicking on the Pod, the card opens the details panel with fade-in and zoom animations and closes it.
The panel fades out and returns to the original set of cards.

Create without filling in the Pod ID, port, or key. Internal IDs are automatically generated; local Pods are created with one click and automatically
Select a stable port and you can rename it after creation. The remote Pod only fills in the Access Point for the first time, and the Server is
Pod details are added one by one, and their internal IDs are also automatically generated. The desktop version automatically generates native Play identity;
The Admin identity of the remote server is configured by the target server. When adding, you need to paste the corresponding Admin
private key.

## Pod type

- Local Pod: The desktop version maintains a local Server, and the port remains stable after creation; the Server
  LAN listening, Admin and Play are still connected from this machine. Front QR code is used in other GizClaw Apps
  Add the Server and enroll with the local App registration credential. A new local Pod contains exactly one
  `RuntimeProfile/default` and creates `RegistrationToken/app:com.gizclaw.opensource`; it does not create Firmware.
  On the back you can start, stop, and restart the Server, and open Admin or Play.
- Remote Pod: Configure zero or more Servers and an Access Point. Admin uses each server
  identity; Play uses Pod-level Client identity to connect to Access Point. Front QR code sharing
  Access Point, on the back maintains the Server list.

The Admin and Play identity of the local Pod are automatically generated. Admin private key of the remote server
The existing configuration from the target server is only retained on the local machine; if it is not filled in, the corresponding Admin remains unconfigured.
Admin and Play will be opened in the system browser after clicking, and the business UI will not be embedded in the Wails window. Pod's
The action menu allows you to edit the declarative configuration, display the directory in the system file manager, or delete the pod after confirmation.

The Server list of remote Pods supports searching by ID, name, and Endpoint. The list uses bounded scrolling and
Virtualization, not when the number of servers is large
Expand to homepage card or system tray.

## Health status

When opening a window, opening Pod details, or manually refreshing, the desktop version accesses the target's `/server-info`,
Displays detecting, reachable, unreachable, or invalid response. Polling will not continue when the window is hidden.

Unresolvable `pod.json` will remain on the home page as a recoverable card with "invalid configuration"; a single bad Pod
Does not prevent other Pods from starting. The original manifest can be repaired by opening its directory from details.

## System Tray

Close, minimize, and maximize/restore buttons are available in the upper left corner of the borderless window. Close button and `Cmd+W` only
Hide the window without stopping the local server or browser HTTP listener. System tray uses identifiable
System icon and provides:

- Open Window;
- Open Pod… for each Pod;
-Quit.

Server, Admin, Play and key operations are all completed in the desktop window without being placed in the tray menu.
Only Quit in the tray actually exits the process and cleans up running resources.
