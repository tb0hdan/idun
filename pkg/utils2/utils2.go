package utils2

func DeduplicateSlice(incoming []string) (outgoing []string) {
	hash := make(map[string]int)
	outgoing = make([]string, 0)
	//
	for _, value := range incoming {
		if _, ok := hash[value]; !ok {
			hash[value] = 1

			outgoing = append(outgoing, value)
		}
	}
	//
	return
}
