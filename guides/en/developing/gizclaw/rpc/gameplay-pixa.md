# Gameplay Assets

`Implementation file: rpc_gameplay_pixa.go`

Handle pixa asset download streaming RPC of Pet and BadgeDef, and provide shared metadata plus binary frame download process.

Gameplay asset selection and permissions belong to `services/gameplay`; this is responsible for RPC stream adaptation.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcGameplayPixaDownloadService` | Pixa download Minimum Gameplay interface required. |
| `handlePetPixaDownload` | Download the pixa of the specified Peer pet. |
| `handleBadgeDefPixaDownload` | Download BadgeDef pixa. |
| `writeRPCDownload` | Uniformly write typed metadata, binary frames and EOS. |
