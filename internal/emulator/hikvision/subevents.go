package hikvision

import "math/rand"

// Subtipos de evento de controle de acesso (majorEventType=5). Espelham
// HikvisionEvents em IoHikvisionCommunication.py:511 — só os que o
// gerenciador realmente consome estão aqui.
const (
	MinorLegalCardPass    = 1  // cartão válido
	MinorCardOutOfDate    = 8  // cartão expirado
	MinorInvalidCard      = 9  // cartão inexistente
	MinorDoorOpenNormal   = 25 // porta aberta (sensor)
	MinorDoorCloseNormal  = 26 // porta fechada (sensor)
	MinorDoorOpenAbnormal = 27 // porta arrombada
	MinorFingerprintPass  = 38 // digital reconhecida
	MinorFingerprintFail  = 39 // digital recusada
	MinorFaceVerifyPass   = 75 // face reconhecida
	MinorFaceVerifyFail   = 76 // face recusada
)

// pesoSubEvento é a distribuição usada no modo standalone. Um equipamento em
// produção concede a esmagadora maioria dos acessos; as recusas existem para
// exercitar os caminhos de INVALID_CARD / BIOMETRIC_VERIFICATION_FAILED /
// CARD_EXPIRED do gerenciador, que antes nunca eram atingidos.
var pesoSubEvento = []struct {
	minor int
	peso  int
}{
	{MinorFaceVerifyPass, 55},
	{MinorLegalCardPass, 15},
	{MinorFingerprintPass, 10},
	{MinorInvalidCard, 8},
	{MinorFaceVerifyFail, 6},
	{MinorFingerprintFail, 3},
	{MinorCardOutOfDate, 3},
}

// pickStandaloneSubEvent sorteia um subEventType de acesso conforme os pesos.
func pickStandaloneSubEvent(r *rand.Rand) int {
	total := 0
	for _, p := range pesoSubEvento {
		total += p.peso
	}
	n := r.Intn(total)
	for _, p := range pesoSubEvento {
		if n < p.peso {
			return p.minor
		}
		n -= p.peso
	}
	return MinorFaceVerifyPass
}

// subEventIsGrant informa se o subtipo representa acesso concedido. O
// gerenciador só chama on_access_authorized nesses três.
func subEventIsGrant(minor int) bool {
	switch minor {
	case MinorLegalCardPass, MinorFingerprintPass, MinorFaceVerifyPass:
		return true
	}
	return false
}
