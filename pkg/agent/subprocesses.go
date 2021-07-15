package agent

import "github.com/mitchellh/go-ps"

func findAllSubprocesses(pid int) []int {
	res := []int{}

	childrenLookup := map[int][]int{}
	processes, err := ps.Processes()
	if err != nil {
		// TODO: handle
		return res
	}
	for _, p := range processes {
		ppid := p.PPid()
		if _, ok := childrenLookup[ppid]; !ok {
			childrenLookup[ppid] = []int{}
		}
		childrenLookup[ppid] = append(childrenLookup[ppid], p.Pid())
	}

	todo := []int{pid}
	for len(todo) > 0 {
		parentPid := todo[0]
		todo = todo[1:]

		if children, ok := childrenLookup[parentPid]; ok {
			for _, childPid := range children {
				res = append(res, childPid)
				todo = append(todo, childPid)
			}
		}
	}

	return res
}
