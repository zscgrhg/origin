package image_ecosystem

import (
	"fmt"
	"time"

	g "github.com/onsi/ginkgo"
	o "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	exutil "github.com/openshift/origin/test/extended/util"
)

var _ = g.Describe("[image_ecosystem][perl][Slow] hot deploy for openshift perl image", func() {
	defer g.GinkgoRecover()
	var (
		dancerTemplate = "https://raw.githubusercontent.com/openshift/dancer-ex/master/openshift/templates/dancer-mysql.json"
		oc             = exutil.NewCLI("s2i-perl", exutil.KubeConfigPath())
		modifyCommand  = []string{"sed", "-ie", `s/data => \$data\[0\]/data => "1337"/`, "lib/default.pm"}
		pageCountFn    = func(count int) string { return fmt.Sprintf(`<span class="code" id="count-value">%d</span>`, count) }
		dcName         = "dancer-mysql-example"
		rcNameOne      = fmt.Sprintf("%s-1", dcName)
		rcNameTwo      = fmt.Sprintf("%s-2", dcName)
		dcLabelOne     = exutil.ParseLabelsOrDie(fmt.Sprintf("deployment=%s", rcNameOne))
		dcLabelTwo     = exutil.ParseLabelsOrDie(fmt.Sprintf("deployment=%s", rcNameTwo))
	)

	g.Describe("Dancer example", func() {
		g.It(fmt.Sprintf("should work with hot deploy"), func() {
			oc.SetOutputDir(exutil.TestContext.OutputDir)

			exutil.CheckOpenShiftNamespaceImageStreams(oc)
			g.By(fmt.Sprintf("calling oc new-app -f %q", dancerTemplate))
			err := oc.Run("new-app").Args("-f", dancerTemplate).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("waiting for build to finish")
			err = exutil.WaitForABuild(oc.Client().Builds(oc.Namespace()), rcNameOne, nil, nil, nil)
			if err != nil {
				exutil.DumpBuildLogs(dcName, oc)
			}
			o.Expect(err).NotTo(o.HaveOccurred())

			// oc.KubeFramework().WaitForAnEndpoint currently will wait forever;  for now, prefacing with our WaitForADeploymentToComplete,
			// which does have a timeout, since in most cases a failure in the service coming up stems from a failed deployment
			err = exutil.WaitForADeploymentToComplete(oc.KubeClient().Core().ReplicationControllers(oc.Namespace()), dcName, oc)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("waiting for endpoint")
			err = oc.KubeFramework().WaitForAnEndpoint(dcName)
			o.Expect(err).NotTo(o.HaveOccurred())

			assertPageCountIs := func(i int, dcLabel labels.Selector) {
				_, err := exutil.WaitForPods(oc.KubeClient().Core().Pods(oc.Namespace()), dcLabel, exutil.CheckPodIsRunningFn, 1, 2*time.Minute)
				o.Expect(err).NotTo(o.HaveOccurred())

				result, err := CheckPageContains(oc, dcName, "", pageCountFn(i))
				o.Expect(err).NotTo(o.HaveOccurred())
				o.Expect(result).To(o.BeTrue())
			}

			g.By("checking page count")
			assertPageCountIs(1, dcLabelOne)
			assertPageCountIs(2, dcLabelOne)

			g.By("modifying the source code with disabled hot deploy")
			RunInPodContainer(oc, dcLabelOne, modifyCommand)
			assertPageCountIs(3, dcLabelOne)

			pods, err := oc.KubeClient().Core().Pods(oc.Namespace()).List(metav1.ListOptions{LabelSelector: dcLabelOne.String()})
			o.Expect(err).NotTo(o.HaveOccurred())
			o.Expect(len(pods.Items)).To(o.Equal(1))

			g.By("turning on hot-deploy")
			err = oc.Run("env").Args("dc", dcName, "PERL_APACHE2_RELOAD=true").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.Run("scale").Args("dc", dcName, "--replicas=0").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())
			err = exutil.WaitUntilPodIsGone(oc.KubeClient().Core().Pods(oc.Namespace()), pods.Items[0].Name, 1*time.Minute)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = oc.Run("scale").Args("dc", dcName, "--replicas=1").Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("modifying the source code with enabled hot deploy")
			assertPageCountIs(4, dcLabelTwo)
			RunInPodContainer(oc, dcLabelTwo, modifyCommand)
			assertPageCountIs(1337, dcLabelTwo)
		})
	})
})
