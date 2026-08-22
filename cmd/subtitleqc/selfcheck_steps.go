package main

import "fmt"

type smokeStep struct {
	Name string
	Run  func() error
}

func runSteps(steps []smokeStep) error {
	for _, s := range steps {
		if s.Run == nil {
			return fmt.Errorf("步骤%s未实现", s.Name)
		}
		if err := s.Run(); err != nil {
			return fmt.Errorf("步骤%s失败: %w", s.Name, err)
		}
	}
	return nil
}
