package currencymath

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateFee(t *testing.T) {
	amount := decimal.RequireFromString("100.005")
	feeRate := decimal.RequireFromString("0.001") // 0.1%

	// Exact is 0.100005
	fee8 := CalculateFee(amount, feeRate, PrecisionCrypto)
	assert.Equal(t, "0.100005", fee8.String())

	// Truncated to 4 decimals: 0.1000
	fee4 := CalculateFee(amount, feeRate, 4)
	assert.Equal(t, "0.1", fee4.String())
}

func TestCalculateSpread(t *testing.T) {
	bid := decimal.RequireFromString("99.9")
	ask := decimal.RequireFromString("100.1")

	// Spread = (100.1 - 99.9) / 100 * 100 = 0.2 / 100 * 100 = 0.2
	spread, err := CalculateSpread(bid, ask, 4)
	require.NoError(t, err)
	assert.Equal(t, "0.2", spread.String())

	// Zero check
	_, err = CalculateSpread(decimal.Zero, ask, 4)
	assert.Error(t, err)
}

func TestValidateMinTradeLimit(t *testing.T) {
	price := decimal.RequireFromString("50000")
	quantity := decimal.RequireFromString("0.0001") // Notional: 5.0
	
	assert.True(t, ValidateMinTradeLimit(price, quantity, decimal.RequireFromString("5.0")))
	assert.False(t, ValidateMinTradeLimit(price, quantity, decimal.RequireFromString("5.01")))
}

func TestDivideWithDust(t *testing.T) {
	tests := []struct {
		name          string
		total         string
		parts         int64
		precision     int32
		expectedPart  string
		expectedDust  string
	}{
		{
			name:         "Clean division",
			total:        "10",
			parts:        2,
			precision:    2,
			expectedPart: "5",
			expectedDust: "0",
		},
		{
			name:         "Repeating decimal 10/3",
			total:        "10",
			parts:        3,
			precision:    2,
			expectedPart: "3.33",
			expectedDust: "0.01",
		},
		{
			name:         "Repeating decimal 10/3 precision 8",
			total:        "10",
			parts:        3,
			precision:    8,
			expectedPart: "3.33333333",
			expectedDust: "0.00000001",
		},
		{
			name:         "Repeating decimal 10/3 precision 18 (Wei)",
			total:        "10",
			parts:        3,
			precision:    18,
			expectedPart: "3.333333333333333333",
			expectedDust: "0.000000000000000001",
		},
		{
			name:         "Small amount precision",
			total:        "0.00000005", // 5 satoshis
			parts:        2,
			precision:    8,
			expectedPart: "0.00000002", // 2 satoshis each
			expectedDust: "0.00000001", // 1 satoshi dust
		},
		{
			name:         "Loss of precision edge case",
			total:        "0.99999999", // 99999999 satoshis
			parts:        100000000,
			precision:    8,
			expectedPart: "0",          // Each gets 0 because 0.99999999 / 100000000 is less than 0.00000001
			expectedDust: "0.99999999", // All is dust
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total := decimal.RequireFromString(tt.total)
			part, dust, err := DivideWithDust(total, tt.parts, tt.precision)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedPart, part.String(), "part mismatch")
			assert.Equal(t, tt.expectedDust, dust.String(), "dust mismatch")

			// Verification: part * parts + dust == total
			sum := part.Mul(decimal.NewFromInt(tt.parts)).Add(dust)
			assert.True(t, sum.Equal(total), "part*parts + dust != total")
		})
	}
}
