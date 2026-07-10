package gizclaw

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type fakePeerHTTPSelfService struct {
	err error
}

func (s fakePeerHTTPSelfService) GetSelfRegistration(context.Context, giznet.PublicKey) (peerhttp.PeerSelf, error) {
	if s.err != nil {
		return peerhttp.PeerSelf{}, s.err
	}
	return peerhttp.PeerSelf{
		PublicKey:          giznet.PublicKey{1}.String(),
		RegistrationStatus: apitypes.PeerRegistrationStatusActive,
	}, nil
}

func (s fakePeerHTTPSelfService) GetSelfRuntime(context.Context, giznet.PublicKey) apitypes.Runtime {
	return apitypes.Runtime{Online: true}
}

type fakePeerHTTPStatusService struct {
	putCalled bool
	putErr    error
}

func (s *fakePeerHTTPStatusService) GetStatus(context.Context, giznet.PublicKey) (apitypes.PeerStatus, error) {
	return apitypes.PeerStatus{}, nil
}

func (s *fakePeerHTTPStatusService) PutStatus(context.Context, giznet.PublicKey, apitypes.PeerStatus) (apitypes.PeerStatus, error) {
	s.putCalled = true
	if s.putErr != nil {
		return apitypes.PeerStatus{}, s.putErr
	}
	return apitypes.PeerStatus{}, nil
}

func TestPeerHTTPPutMeStatusChecksPeerBeforeWrite(t *testing.T) {
	status := &fakePeerHTTPStatusService{}
	svc := &peerHTTP{
		Self:   fakePeerHTTPSelfService{err: peer.ErrPeerNotFound},
		Status: status,
	}
	ctx := peerhttp.WithCallerPublicKey(context.Background(), giznet.PublicKey{1})
	resp, err := svc.PutMeStatus(ctx, peerhttp.PutMeStatusRequestObject{Body: &apitypes.PeerStatus{}})
	if err != nil {
		t.Fatalf("PutMeStatus error = %v", err)
	}
	if _, ok := resp.(peerhttp.PutMeStatus404JSONResponse); !ok {
		t.Fatalf("PutMeStatus response = %T, want 404", resp)
	}
	if status.putCalled {
		t.Fatal("PutStatus called for missing peer")
	}
}

func TestPeerHTTPPutMeStatusErrorMapping(t *testing.T) {
	ctx := peerhttp.WithCallerPublicKey(context.Background(), giznet.PublicKey{1})
	for _, tc := range []struct {
		name string
		err  error
		want any
	}{
		{name: "validation", err: peerrun.ErrInvalidStatus, want: peerhttp.PutMeStatus400JSONResponse{}},
		{name: "internal", err: peerrun.ErrNilStore, want: peerhttp.PutMeStatus500JSONResponse{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &peerHTTP{
				Self:   fakePeerHTTPSelfService{},
				Status: &fakePeerHTTPStatusService{putErr: tc.err},
			}
			resp, err := svc.PutMeStatus(ctx, peerhttp.PutMeStatusRequestObject{Body: &apitypes.PeerStatus{}})
			if err != nil {
				t.Fatalf("PutMeStatus error = %v", err)
			}
			switch tc.want.(type) {
			case peerhttp.PutMeStatus400JSONResponse:
				if _, ok := resp.(peerhttp.PutMeStatus400JSONResponse); !ok {
					t.Fatalf("PutMeStatus response = %T, want 400", resp)
				}
			case peerhttp.PutMeStatus500JSONResponse:
				if _, ok := resp.(peerhttp.PutMeStatus500JSONResponse); !ok {
					t.Fatalf("PutMeStatus response = %T, want 500", resp)
				}
			default:
				t.Fatalf("unhandled want type %T", tc.want)
			}
		})
	}
}
