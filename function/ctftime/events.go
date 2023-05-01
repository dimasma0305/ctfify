package ctftime

type Events []*Event

func (events *Events) Filter(f func(chall *Event) bool) Events {
	var res []*Event
	for _, v := range *events {
		if f(v) {
			res = append(res, v)
		}
	}
	return res
}
