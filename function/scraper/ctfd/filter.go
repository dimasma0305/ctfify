package ctfd

type ChallengesInfo []*ChallengeInfo

func (ac ChallengesInfo) Filter(f func(chall *ChallengeInfo) bool) []*ChallengeInfo {
	var res []*ChallengeInfo
	for _, v := range ac {
		if f(v) {
			res = append(res, v)
		}
	}
	return res
}
