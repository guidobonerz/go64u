package helper

import "strconv"

func ParseAddressAndValue(addressStr, valueStr string) (int, int, error) {
	address, err := getHexValue(addressStr)
	if err != nil {
		return 0, 0, err
	}

	value, err := getHexValue(valueStr)
	if err != nil {
		return 0, 0, err
	}

	return int(address), int(value), nil
}

func getHexValue(hexValue string) (int64, error) {
	value, _ := strconv.ParseInt(hexValue, 16, 64)
	return value, nil
}
