package currencymath

import (
	"errors"
	"math/big"

	"github.com/shopspring/decimal"
)

// Precision constants defined for standard crypto/fiat usage
const (
	PrecisionFiat    int32 = 2
	PrecisionCrypto  int32 = 8
	PrecisionWei     int32 = 18
)

// CalculateFee strictly computes the exchange fee using multiplication and truncation.
// Truncation (rounding towards zero) ensures we do not overcharge users or create
// money out of thin air due to rounding. Any lost micro-dust remains with the user.
func CalculateFee(amount, feeRate decimal.Decimal, precision int32) decimal.Decimal {
	fee := amount.Mul(feeRate)
	return fee.Truncate(precision)
}

// CalculateSpread calculates the percentage spread (bid-ask spread) based on the mid price.
// Formula: Spread% = (Ask - Bid) / ((Ask + Bid) / 2) * 100
func CalculateSpread(bid, ask decimal.Decimal, precision int32) (decimal.Decimal, error) {
	if bid.IsZero() || ask.IsZero() {
		return decimal.Zero, errors.New("prices cannot be zero")
	}
	
	two := decimal.NewFromInt(2)
	midPrice := bid.Add(ask).DivRound(two, precision+8)
	
	if midPrice.IsZero() {
		return decimal.Zero, errors.New("mid price is zero")
	}

	hundred := decimal.NewFromInt(100)
	spreadRaw := ask.Sub(bid).DivRound(midPrice, precision+8).Mul(hundred)
	
	return spreadRaw.Truncate(precision), nil
}

// ValidateMinTradeLimit checks if a trade strictly passes the dynamic minimum notional value requirement.
func ValidateMinTradeLimit(price, quantity, minNotional decimal.Decimal) bool {
	notional := price.Mul(quantity)
	return notional.GreaterThanOrEqual(minNotional)
}

// DivideWithDust safely splits a total amount into a specified number of parts up to a certain precision.
// It deterministically rounds DOWN the exact quotient using BigInt integer division to completely avoid
// IEEE 754 float approximations or DivRound edge cases that could invent fractions of a satoshi.
// Returns the individual part and the remaining unallocated dust balance to be sent to a DustRevenueAccount.
func DivideWithDust(totalAmount decimal.Decimal, parts int64, precision int32) (part decimal.Decimal, dust decimal.Decimal, err error) {
	if parts <= 0 {
		return decimal.Zero, decimal.Zero, errors.New("parts must be strictly positive")
	}
	if totalAmount.IsNegative() {
		return decimal.Zero, decimal.Zero, errors.New("total amount cannot be negative")
	}

	// Mathematical pure truncation using BigInt:
	// We want floor(totalAmount / parts) at 'precision' decimal places.
	// Formula: BigInt(totalAmount * 10^precision) / parts
	multiplier := decimal.NewFromInt(10).Pow(decimal.NewFromInt(int64(precision)))
	
	// Convert total * 10^precision into a big.Int to strip out any remaining fraction and do pure integer math.
	scaledTotal := totalAmount.Mul(multiplier).BigInt()
	
	bigParts := big.NewInt(parts)
	bigPartQuo := new(big.Int).Div(scaledTotal, bigParts)
	
	// Reconstruct the decimal value from the truncated bigInt quotient
	part = decimal.NewFromBigInt(bigPartQuo, -precision)

	// Calculate dust safely using exact Decimal arithmetic
	partsDec := decimal.NewFromInt(parts)
	sumParts := part.Mul(partsDec)
	
	if sumParts.GreaterThan(totalAmount) {
		return decimal.Zero, decimal.Zero, errors.New("fatal rounding violation: sum of parts exceeded total amount")
	}

	dust = totalAmount.Sub(sumParts)
	return part, dust, nil
}
