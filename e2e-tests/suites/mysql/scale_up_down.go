// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	"github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	secret_utils "github.com/mysql/ndb-operator/e2e-tests/utils/secret"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = ndbtest.NewTestCase("MySQL Servers scaling up and down", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var ndbName, mysqlRootSecretName string
	var testNdb *v1alpha1.NdbCluster

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()

		ginkgo.By("Deploying operator in namespace'" + ns + "'")
		ndbtest.DeployNdbOperator(c, ns)
	})

	ginkgo.AfterEach(func() {
		ginkgo.By("Deleting ndb operator and other resources")
		ndbtest.UndeployNdbOperator(c, ns)
	})

	ginkgo.When("mysqld.nodeCount is updated", func() {

		ginkgo.BeforeEach(func() {
			ndbName = "ndb-mysqld-test"
			mysqlRootSecretName = ndbName + "-root-secret"
			// create the secret first
			secret_utils.CreateSecretForMySQLRootAccount(c, mysqlRootSecretName, ns)
			// create the Ndb resource
			testNdb = testutils.NewTestNdb(ns, ndbName, 2)
			testNdb.Spec.Mysqld.RootPasswordSecretName = mysqlRootSecretName
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.AfterEach(func() {
			// cleanup
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
			// drop the secret
			secret_utils.DeleteSecret(c, mysqlRootSecretName, ns)
		})

		ginkgo.It("should scale up or scale down the MySQL Servers", func() {

			ginkgo.By("verifying the initial MySQL Server node count and running queries", func() {
				deployment.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 2)
				db := mysql.Connect(c, testNdb, "")
				_, err := db.Exec("create database test")
				ndbtest.ExpectNoError(err, "create database test failed")
				_, err = db.Exec("create table test.t1 (id int, value char(10)) engine ndb")
				ndbtest.ExpectNoError(err, "create table t1 failed")
			})

			ginkgo.By("scaling up the MySQL Servers", func() {
				testNdb.Spec.Mysqld.NodeCount = 5
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			ginkgo.By("verifying the MySQL Server node count after scale up and running queries", func() {
				deployment.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 5)
				db := mysql.Connect(c, testNdb, "test")
				result, err := db.Exec("insert into t1 values (1, 'ndb'), (2, 'operator')")
				ndbtest.ExpectNoError(err, "insert into t1 failed")
				gomega.Expect(result.RowsAffected()).To(gomega.Equal(int64(2)))
			})

			ginkgo.By("scaling down the MySQL Servers", func() {
				testNdb.Spec.Mysqld.NodeCount = 1
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			ginkgo.By("verifying the MySQL Server node count after scale up and running queries", func() {
				deployment.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 1)
				db := mysql.Connect(c, testNdb, "test")
				row := db.QueryRow("select value from t1 where id = 2")
				var value string
				ndbtest.ExpectNoError(row.Scan(&value), "select value from t1 failed")
				gomega.Expect(value).To(gomega.Equal("operator"),
					"'select' query returned unexpected value")
			})
		})
	})
})
