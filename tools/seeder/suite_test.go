package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSeeder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Seeder Suite")
}
