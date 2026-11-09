package wameow

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"google.golang.org/protobuf/proto"
)

func buildHistoryRequest(id string, now time.Time, days int) *waE2E.Message {
	config := proto.Clone(store.DeviceProps.HistorySyncConfig).(*waCompanionReg.DeviceProps_HistorySyncConfig)
	config.FullSyncDaysLimit = proto.Uint32(uint32(days))
	return &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
		PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
			PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_FULL_HISTORY_SYNC_ON_DEMAND.Enum(),
			FullHistorySyncOnDemandRequest: &waE2E.PeerDataOperationRequestMessage_FullHistorySyncOnDemandRequest{
				RequestMetadata:               &waE2E.FullHistorySyncOnDemandRequestMetadata{RequestID: proto.String(id)},
				HistorySyncConfig:             config,
				FullHistorySyncOnDemandConfig: &waE2E.FullHistorySyncOnDemandConfig{HistoryFromTimestamp: proto.Uint64(uint64(now.AddDate(0, 0, -days).Unix())), HistoryDurationDays: proto.Uint32(uint32(days))},
			},
		},
	}}
}

// A successful send acknowledges only the request, not delivery of history.
// WhatsApp may refuse or limit the transfer; HistorySync handles any response data.
// days: quantos dias para trás o celular deve enviar (o WhatsApp pode entregar menos que o pedido).
func (p *Provider) RequestHistory(ctx context.Context, days int) error {
	cli, err := p.client()
	if err != nil {
		return err
	}
	_, err = cli.SendPeerMessage(ctx, buildHistoryRequest(p.NewMessageID(), time.Now(), days))
	return err
}
