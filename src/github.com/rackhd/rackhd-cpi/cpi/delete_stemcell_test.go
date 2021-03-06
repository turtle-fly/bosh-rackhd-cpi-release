package cpi_test

import (
	"fmt"
	"net/http"

	"github.com/rackhd/rackhd-cpi/bosh"
	"github.com/rackhd/rackhd-cpi/config"
	"github.com/rackhd/rackhd-cpi/cpi"
	"github.com/rackhd/rackhd-cpi/helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DeleteStemcell", func() {
	Context("with valid CPI v1 input", func() {
		It("deletes a previously uploaded stemcell from the rackhd server", func() {
			apiServer, err := helpers.GetRackHDHost()
			Expect(err).ToNot(HaveOccurred())

			c := config.Cpi{ApiServer: apiServer}

			var createInput bosh.MethodArguments
			createInput = append(createInput, "../spec_assets/image")

			baseName, err := cpi.CreateStemcell(c, createInput)
			Expect(err).ToNot(HaveOccurred())

			var deleteInput bosh.MethodArguments
			deleteInput = append(deleteInput, baseName)
			err = cpi.DeleteStemcell(c, deleteInput)
			Expect(err).ToNot(HaveOccurred())

			url := fmt.Sprintf("%s/api/common/files/metadata/%s", c.ApiServer, baseName)
			resp, err := http.Get(url)
			Expect(err).ToNot(HaveOccurred())

			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(404))
		})
	})

	Context("with invalid CPI v1 input", func() {
		It("returns an error", func() {
			apiServer, err := helpers.GetRackHDHost()
			Expect(err).ToNot(HaveOccurred())

			c := config.Cpi{ApiServer: apiServer}

			var deleteInput bosh.MethodArguments
			deleteInput = append(deleteInput, map[string]string{"invalid": "true"})
			err = cpi.DeleteStemcell(c, deleteInput)
			Expect(err).To(MatchError("Received unexpected type for stemcell cid"))
		})
	})
})
