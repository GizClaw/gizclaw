package ogg

var oggCRCTable = buildOGGCRCTable()

func buildOGGCRCTable() [256]uint32 {
	const polynomial uint32 = 0x04c11db7
	var table [256]uint32
	for i := range 256 {
		crc := uint32(i) << 24
		for range 8 {
			if (crc & (1 << 31)) != 0 {
				crc = (crc << 1) ^ polynomial
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}

func oggCRC(data []byte) uint32 {
	crc := uint32(0)
	for _, b := range data {
		crc = (crc << 8) ^ oggCRCTable[byte((crc>>24)^uint32(b))]
	}
	return crc
}
