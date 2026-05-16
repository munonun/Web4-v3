package message

type MessageType string

const (
	TypeHello           MessageType = "HELLO"
	TypeQuoteRequest    MessageType = "QUOTE_REQUEST"
	TypeQuoteResponse   MessageType = "QUOTE_RESPONSE"
	TypeSignedIntent    MessageType = "SIGNED_INTENT"
	TypeAuthorizedTrade MessageType = "AUTHORIZED_TRADE"
	TypeTradeResult     MessageType = "TRADE_RESULT"
	TypeReject          MessageType = "REJECT"
	TypePing            MessageType = "PING"
	TypePong            MessageType = "PONG"
)

func IsValidMessageType(t MessageType) bool {
	switch t {
	case TypeHello,
		TypeQuoteRequest,
		TypeQuoteResponse,
		TypeSignedIntent,
		TypeAuthorizedTrade,
		TypeTradeResult,
		TypeReject,
		TypePing,
		TypePong:
		return true
	default:
		return false
	}
}
