package bundle

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhat-openshift-ecosystem/openshift-preflight/certification"
)

var _ = Describe("BundleValidateCheck", func() {
	const (
		clusterServiceVersionFilename = "myoperator.clusterserviceversion.yaml"
		manifestsDir                  = "manifests"
		metadataDir                   = "metadata"
		annotationFilename            = "annotations.yaml"
		annotations                   = `annotations:
  com.redhat.openshift.versions: "v4.6-v4.9"
  operators.operatorframework.io.bundle.package.v1: testPackage
  operators.operatorframework.io.bundle.channel.default.v1: testChannel
`
		csvContents = `spec:
  install:
    spec:
      deployments:
      - spec:
          template:
            spec:
              containers:
              - image: registry.example.io/foo/bar@sha256:f000432f07cd187469f0310e3ed9dcf9a5db2be14b8bab9c5293dd1ee8518176

  relatedImages:
  - name: the-operator
    image: registry.example.io/foo/bar@sha256:f000432f07cd187469f0310e3ed9dcf9a5db2be14b8bab9c5293dd1ee8518176
  - name: the-proxy
    image: registry.example.io/foo/proxy@sha256:5e33f9d095952866b9743cc8268fb740cce6d93439f00ce333a2de1e5974837e`
	)

	Describe("Bundle validation", func() {
		var (
			imageRef   certification.ImageReference
			fakeEngine operatorSdk
		)

		BeforeEach(func() {
			// mock bundle directory
			tmpDir, err := os.MkdirTemp("", "bundle-metadata-*")
			Expect(err).ToNot(HaveOccurred())

			err = os.Mkdir(filepath.Join(tmpDir, metadataDir), 0o755)
			Expect(err).ToNot(HaveOccurred())

			err = os.Mkdir(filepath.Join(tmpDir, manifestsDir), 0o755)
			Expect(err).ToNot(HaveOccurred())

			err = os.WriteFile(filepath.Join(tmpDir, metadataDir, annotationFilename), []byte(annotations), 0o644)
			Expect(err).ToNot(HaveOccurred())

			imageRef.ImageFSPath = tmpDir
			fakeEngine = FakeOperatorSdk{}
		})

		AfterEach(func() {
			err := os.RemoveAll(imageRef.ImageFSPath)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("the annotations file is valid", func() {
			It("should pass", func() {
				report, err := Validate(context.Background(), fakeEngine, imageRef.ImageFSPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(report).ToNot(BeNil())
			})
		})

		Context("the annotations file does not exist", func() {
			JustBeforeEach(func() {
				err := os.Remove(filepath.Join(imageRef.ImageFSPath, metadataDir, annotationFilename))
				Expect(err).ToNot(HaveOccurred())
			})
			It("should error", func() {
				report, err := Validate(context.Background(), fakeEngine, imageRef.ImageFSPath)
				Expect(err).To(HaveOccurred())
				Expect(report).To(BeNil())
			})
		})

		Context("the annotations file is malformed", func() {
			JustBeforeEach(func() {
				err := os.WriteFile(filepath.Join(imageRef.ImageFSPath, metadataDir, annotationFilename), []byte("badAnnotations"), 0o644)
				Expect(err).ToNot(HaveOccurred())
			})
			It("should error", func() {
				report, err := Validate(context.Background(), fakeEngine, imageRef.ImageFSPath)
				Expect(err).To(HaveOccurred())
				Expect(report).To(BeNil())
			})
		})

		Context("the annotations file is valid but has no annotations", func() {
			JustBeforeEach(func() {
				err := os.WriteFile(filepath.Join(imageRef.ImageFSPath, metadataDir, annotationFilename), []byte("annotations:"), 0o644)
				Expect(err).ToNot(HaveOccurred())
			})
			It("should fail gracefully", func() {
				report, err := Validate(context.Background(), fakeEngine, imageRef.ImageFSPath)
				Expect(err).ToNot(HaveOccurred())
				Expect(report).ToNot(BeNil())
			})
		})

		Context("getting the CSV file from the bundle", func() {
			var manifestsPath string

			BeforeEach(func() {
				manifestsPath = filepath.Join(imageRef.ImageFSPath, manifestsDir)
				err := os.WriteFile(filepath.Join(manifestsPath, clusterServiceVersionFilename), []byte(""), 0o644)
				Expect(err).ToNot(HaveOccurred())
			})
			Context("the CSV contains images", func() {
				It("should get the images", func() {
					r := strings.NewReader(csvContents)
					images, err := ExtractImagesFromBundle(context.TODO(), r)
					Expect(err).ToNot(HaveOccurred())
					Expect(images).To(HaveLen(2))
					Expect(images).To(ContainElement("registry.example.io/foo/bar@sha256:f000432f07cd187469f0310e3ed9dcf9a5db2be14b8bab9c5293dd1ee8518176"))
				})
			})
			Context("the CSV is malformed", func() {
				It("should error", func() {
					r := strings.NewReader("badcsv::bad")
					images, err := ExtractImagesFromBundle(context.TODO(), r)
					Expect(err).To(HaveOccurred())
					Expect(images).To(BeNil())
				})
			})
			Context("the CSV could not be read", func() {
				It("should error", func() {
					images, err := ExtractImagesFromBundle(context.TODO(), errReader(0))
					Expect(err).To(HaveOccurred())
					Expect(images).To(BeNil())
				})
			})
			Context("the CSV exists by itself", func() {
				It("should return the filename", func() {
					filename, err := GetCsvFilePathFromBundle(imageRef.ImageFSPath)
					Expect(err).ToNot(HaveOccurred())
					Expect(filename).To(Equal(filepath.Join(manifestsPath, clusterServiceVersionFilename)))
				})
			})
			Context("the CSV doesn't exist", func() {
				JustBeforeEach(func() {
					err := os.Remove(filepath.Join(manifestsPath, clusterServiceVersionFilename))
					Expect(err).ToNot(HaveOccurred())
				})
				It("should return an error", func() {
					filename, err := GetCsvFilePathFromBundle(imageRef.ImageFSPath)
					Expect(err).To(HaveOccurred())
					Expect(filename).To(Equal(""))
				})
			})
			Context("there is more than one CSV", func() {
				JustBeforeEach(func() {
					err := os.WriteFile(filepath.Join(manifestsPath, "otheroperator.clusterserviceversion.yaml"), []byte(""), 0o664)
					Expect(err).ToNot(HaveOccurred())
				})
				It("should return an error", func() {
					filename, err := GetCsvFilePathFromBundle(imageRef.ImageFSPath)
					Expect(err).To(HaveOccurred())
					Expect(filename).To(Equal(""))
				})
			})
			Context("there is a bad mount dir", func() {
				It("should return an error", func() {
					filename, err := GetCsvFilePathFromBundle("[]")
					Expect(err).To(HaveOccurred())
					Expect(filename).To(Equal(""))
				})
			})
		})
	})

	Describe("Supported Install Modes", func() {
		var csv string = `spec:
  installModes:
  - supported: true
    type: OwnNamespace
  - supported: true
    type: SingleNamespace
  - supported: false
    type: MultiNamespace
  - supported: true
    type: AllNamespaces`

		Context("CSV is valid", func() {
			It("should return a map of 3", func() {
				installModes, err := GetSupportedInstallModes(context.Background(), strings.NewReader(csv))
				Expect(err).ToNot(HaveOccurred())
				Expect(installModes).ToNot(BeNil())
				Expect(len(installModes)).To(Equal(3))
				Expect("MultiNamespace").ToNot(BeElementOf(installModes))
			})
		})

		Context("reader is not valid", func() {
			It("should error", func() {
				installModes, err := GetSupportedInstallModes(context.Background(), errReader(0))
				Expect(err).To(HaveOccurred())
				Expect(installModes).To(BeNil())
			})
		})

		Context("CSV is invalid", func() {
			JustBeforeEach(func() {
				csv = `invalid`
			})
			It("should error", func() {
				installModes, err := GetSupportedInstallModes(context.Background(), strings.NewReader(csv))
				Expect(err).To(HaveOccurred())
				Expect(installModes).To(BeNil())
			})
		})
	})

	Describe("While ensuring that container util is working", func() {
		// tests: extractAnnotationsBytes
		Context("with an annotations yaml data read from disk", func() {
			Context("with the correct format", func() {
				data := []byte("annotations:\n foo: bar")

				It("should properly marshal to a map[string]string", func() {
					annotations, err := ExtractAnnotationsBytes(context.TODO(), data)
					Expect(err).ToNot(HaveOccurred())
					Expect(annotations["foo"]).To(Equal("bar"))
				})
			})

			Context("containing no data read in from the yaml file", func() {
				data := []byte{}

				It("should return an error", func() {
					_, err := ExtractAnnotationsBytes(context.TODO(), data)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("containing malformed or unexpected data", func() {
				data := []byte(`malformed`)

				It("should return an error", func() {
					_, err := ExtractAnnotationsBytes(context.TODO(), data)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("a bad reader is sent to GetAnnotations", func() {
				It("should return an error", func() {
					annotations, err := GetAnnotations(context.Background(), errReader(0))
					Expect(err).To(HaveOccurred())
					Expect(annotations).To(BeNil())
				})
			})
		})
	})

	DescribeTable("Image Registry validation",
		func(versions string, expected bool) {
			ok := isTarget49OrGreater(versions)
			Expect(ok).To(Equal(expected))
		},
		Entry("range 4.6 to 4.8", "v4.6-v4.8", false),
		Entry("exactly 4.8", "=v4.8", false),
		Entry("exactly 4.9", "=v4.9", true),
		Entry("range 4.6 to 4.9", "v4.6-v4.9", true),
		Entry(">= 4.8", "v4.8", true),
		Entry(">= 4.9", "v4.9", true),
		Entry("begins = with error", "=foo", false),
		Entry("bare version with error", "vfoo", false),
		Entry("range with error", "v4.6-vfoo", false),
	)
})
