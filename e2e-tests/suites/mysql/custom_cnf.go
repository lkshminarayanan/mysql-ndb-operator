package e2e

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"time"

	"github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
)

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	// create the Ndb CRD to be used by the suite
	ndbtest.CreateNdbCRD()
	return nil
},
	func([]byte) {},
)

var _ = ginkgo.SynchronizedAfterSuite(func() {
	// delete the Ndb CRD once the suite is done running
	ndbtest.DeleteNdbCRD()
}, func() {}, 5000)

var _ = ndbtest.DescribeFeature("MySQL Custom cnf", func() {
	var ns string
	var c clientset.Interface

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from framework")
		f := ndbtest.GetFramework()
		ns = f.Namespace.Name
		c = f.ClientSet

		ginkgo.By("Deploying operator in namespace'" + ns + "'")
		ndbtest.DeployNdbOperator(c, ns)
	})

	ginkgo.AfterEach(func() {
		ginkgo.By("Deleting ndb operator and other resources")
		ndbtest.UndeployNdbOperator(c, ns)
	})

	ginkgo.When("a custom cnf property is specified for MySQL Server", func() {

		ginkgo.BeforeEach(func() {
			ndbtest.CreateNdbResource(c, ns, "artifacts/examples", "example-ndb-cnf")
		})

		ginkgo.AfterEach(func() {
			ndbtest.DeleteNdbResource(c, ns, "example-ndb", "artifacts/examples", "example-ndb-cnf")
		})

		ginkgo.It("should start the server with those values as the defaults", func() {
			// TODO : Implement MySQL Readiness Probes to skip timed waiting
			time.Sleep(2 * time.Minute)
			db := mysql.Connect(c, ns, "example-ndb", "performance_schema")

			ginkgo.By("verifying that max_user_connections is properly set in server")
			row := db.QueryRow(
				"select variable_value from global_variables where variable_name = 'max_user_connections';")
			var value int
			framework.ExpectNoError(row.Scan(&value),
				"querying for max_user_connections returned an error")
			gomega.Expect(value).To(gomega.Equal(42),
				"max_user_connections had an unexpected value")

			ginkgo.By("verifying that the defaults doesn't override the value set by the operator")
			row = db.QueryRow(
				"select variable_value from global_variables where variable_name = 'ndb_extra_logging';")
			framework.ExpectNoError(row.Scan(&value),
				"querying for ndb_extra_logging returned an error")
			gomega.Expect(value).To(gomega.Equal(99),
				"ndb_extra_logging had an unexpected value")
		})
	})
})
