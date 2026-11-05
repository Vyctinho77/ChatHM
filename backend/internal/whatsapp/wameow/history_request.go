package wameow

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"google.golang.org/protobuf/proto"
)

func buildHistoryRequest(id string, now time.Time) *waE2E.Message {
	config := proto.Clone(store.DeviceProps.HistorySyncConfig).(*waCompanionReg.DeviceProps_HistorySyncConfig)
	config.FullSyncDaysLimit = proto.Uint32(30)
	return &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
		PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
			PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_FULL_HISTORY_SYNC_ON_DEMAND.Enum(),
			FullHistorySyncOnDemandRequest: &waE2E.PeerDataOperationRequestMessage_FullHistorySyncOnDemandRequest{
				RequestMetadata:               &waE2E.FullHistorySyncOnDemandRequestMetadata{RequestID: proto.String(id)},
				HistorySyncConfig:             config,
				FullHistorySyncOnDemandConfig: &waE2E.FullHistorySyncOnDemandConfig{HistoryFromTimestamp: proto.Uint64(uint64(now.AddDate(0, 0, -30).Unix())), HistoryDurationDays: proto.Uint32(30)},
			},
		},
	}}
}

// A successful send acknowledges only the request, not delivery of history.
// WhatsApp may refuse or limit the transfer; HistorySync handles any response data.
func (p *Provider) RequestHistory(ctx context.Context) error {
	cli, err := p.client()
	if err != nil {
		return err
	}
	_, err = cli.SendPeerMessage(ctx, buildHistoryRequest(p.NewMessageID(), time.Now()))
	return err
}
