/*
Copyright The Stash Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package framework

import (
	"database/sql"
	"fmt"
	"time"

	"stash.appscode.dev/apimachinery/apis"
	"stash.appscode.dev/apimachinery/apis/stash/v1alpha1"
	"stash.appscode.dev/apimachinery/apis/stash/v1beta1"
	"stash.appscode.dev/apimachinery/pkg/docker"

	"github.com/appscode/go/sets"
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	apps_util "kmodules.xyz/client-go/apps/v1"
	appCatalog "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
)

const (
	KeyUser     = "username"
	KeyPassword = "password"
	SuperUser   = "root"

	KeyMySQLRootPassword   = "MYSQL_ROOT_PASSWORD"
	MySQLServingPortName   = "mysql"
	MySQLContainerName     = "mysql"
	MySQLServingPortNumber = 3306
	MySQLBackupTask        = "mysql-backup-8.0.14"
	MySQLRestoreTask       = "mysql-restore-8.0.14"
	MySQLBackupFunction    = "mysql-backup-8.0.14"
	MySQLRestoreFunction   = "mysql-restore-8.0.14"
)

func (fi *Invocation) MySQLCredentials() *core.Secret {
	name := fmt.Sprintf("mysql-%s", fi.app)
	return &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fi.namespace,
		},
		Data: map[string][]byte{
			KeyUser:     []byte(SuperUser),
			KeyPassword: []byte(fi.app),
		},
		Type: core.SecretTypeOpaque,
	}
}

func (fi *Invocation) MySQLService() *core.Service {
	name := fmt.Sprintf("mysql-%s", fi.app)
	return &core.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fi.namespace,
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{
				"app": name,
			},
			Ports: []core.ServicePort{
				{
					Name: MySQLServingPortName,
					Port: MySQLServingPortNumber,
				},
			},
		},
	}
}

func (fi *Invocation) MySQLPVC() *core.PersistentVolumeClaim {
	name := fmt.Sprintf("mysql-%s", fi.app)
	return &core.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fi.namespace,
		},
		Spec: core.PersistentVolumeClaimSpec{
			AccessModes: []core.PersistentVolumeAccessMode{
				core.ReadWriteOnce,
			},
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceStorage: resource.MustParse("128Mi"),
				},
			},
		},
	}
}

func (fi *Invocation) MySQLDeployment(cred *core.Secret, pvc *core.PersistentVolumeClaim) *apps.Deployment {
	name := fmt.Sprintf("mysql-%s", fi.app)
	label := map[string]string{
		"app": name,
	}
	return &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fi.namespace,
		},
		Spec: apps.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: label,
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: label,
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name:  MySQLContainerName,
							Image: "mysql:8.0.14",
							Env: []core.EnvVar{
								{
									Name: KeyMySQLRootPassword,
									ValueFrom: &core.EnvVarSource{
										SecretKeyRef: &core.SecretKeySelector{
											LocalObjectReference: core.LocalObjectReference{
												Name: cred.Name,
											},
											Key: KeyPassword,
										},
									},
								},
							},
							Ports: []core.ContainerPort{
								{
									Name:          MySQLServingPortName,
									ContainerPort: MySQLServingPortNumber,
								},
							},
							VolumeMounts: []core.VolumeMount{
								{
									Name:      pvc.Name,
									MountPath: "/var/lib/mysql",
								},
								{
									Name:      "config-volume",
									MountPath: "/etc/mysql/conf.d",
								},
							},
						},
					},
					Volumes: []core.Volume{
						{
							Name: pvc.Name,
							VolumeSource: core.VolumeSource{
								PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
									ClaimName: pvc.Name,
								},
							},
						},
						{
							Name: "config-volume",
							VolumeSource: core.VolumeSource{
								EmptyDir: &core.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
}

func (fi *Invocation) MySQLAppBinding(cred *core.Secret, svc *core.Service) *appCatalog.AppBinding {
	name := fmt.Sprintf("mysql-%s", fi.app)
	return &appCatalog.AppBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fi.namespace,
		},
		Spec: appCatalog.AppBindingSpec{
			Type:    "mysql",
			Version: "8.0.14",
			ClientConfig: appCatalog.ClientConfig{
				Service: &appCatalog.ServiceReference{
					Scheme: "mysql",
					Name:   svc.Name,
					Port:   MySQLServingPortNumber,
				},
			},
			Secret: &core.LocalObjectReference{
				Name: cred.Name,
			},
		},
	}
}

func (fi *Invocation) DeployMySQLDatabase() (*apps.Deployment, *appCatalog.AppBinding, error) {
	By("Creating Secret for MySQL")
	cred := fi.MySQLCredentials()
	_, err := fi.CreateSecret(*cred)
	Expect(err).NotTo(HaveOccurred())

	By("Creating PVC for MySQL")
	pvc := fi.MySQLPVC()
	_, err = fi.CreatePersistentVolumeClaim(pvc)
	Expect(err).NotTo(HaveOccurred())

	By("Creating Service for MySQL")
	svc := fi.MySQLService()
	_, err = fi.CreateService(*svc)
	Expect(err).NotTo(HaveOccurred())

	By("Creating MySQL")
	dpl := fi.MySQLDeployment(cred, pvc)
	dpl, err = fi.CreateDeployment(*dpl)
	Expect(err).NotTo(HaveOccurred())

	By("Waiting for MySQL Deployment to be ready")
	err = apps_util.WaitUntilDeploymentReady(fi.KubeClient, dpl.ObjectMeta)
	Expect(err).NotTo(HaveOccurred())

	By("Creating AppBinding for the MySQL")
	appBinding := fi.MySQLAppBinding(cred, svc)
	appBinding, err = fi.createAppBinding(appBinding)
	Expect(err).NotTo(HaveOccurred())

	fi.AppendToCleanupList(appBinding, dpl, svc, pvc, cred)
	return dpl, appBinding, nil
}

func (fi *Invocation) EventuallyConnectWithMySQLServer(db *sql.DB) error {

	return wait.PollImmediate(5*time.Second, 5*time.Minute, func() (bool, error) {
		if err := db.Ping(); err != nil {
			return false, nil // don't return error. we need to retry.
		}
		return true, nil
	})
}

func (fi *Invocation) createAppBinding(appBinding *appCatalog.AppBinding) (*appCatalog.AppBinding, error) {
	return fi.catalogClient.AppcatalogV1alpha1().AppBindings(appBinding.Namespace).Create(appBinding)
}

func (fi *Invocation) CreateTable(db *sql.DB, tableName string) error {
	stmnt, err := db.Prepare(fmt.Sprintf("CREATE TABLE %s ( property varchar(25),  value int );", tableName))
	if err != nil {
		return err
	}
	defer stmnt.Close()

	_, err = stmnt.Exec()
	return err
}

func (fi *Invocation) ListTables(db *sql.DB) (sets.String, error) {
	res, err := db.Query("SHOW TABLES IN mysql")
	if err != nil {
		return nil, err
	}
	defer res.Close()
	tables := sets.String{}
	var tableName string
	for res.Next() {
		err = res.Scan(&tableName)
		if err != nil {
			return nil, err
		}
		tables.Insert(tableName)
	}
	return tables, nil
}

func (fi *Invocation) InsertRow(db *sql.DB, tableName string, property string, value int) error {
	stmnt, err := db.Prepare(fmt.Sprintf("INSERT INTO %s( property, value) VALUES(?,?);", tableName))
	if err != nil {
		return err
	}
	defer stmnt.Close()

	_, err = stmnt.Exec(property, value)
	return err
}

func (fi *Invocation) ReadProperty(db *sql.DB, tableName, property string) (int, error) {
	res, err := db.Query(fmt.Sprintf("SELECT * FROM %s WHERE property=?;", tableName), property)
	if err != nil {
		return 0, err
	}
	defer res.Close()

	var propertyName string
	var value int

	for res.Next() {
		err = res.Scan(&propertyName, &value)
		if err != nil {
			return 0, err
		}
		if propertyName == property {
			return value, nil
		}
	}
	return 0, fmt.Errorf("no entry for property: %q in the database", property)
}

func (fi *Invocation) UpdateProperty(db *sql.DB, tableName, property string, newValue int) error {
	stmnt, err := db.Prepare(fmt.Sprintf("UPDATE %s SET value=? WHERE property=?; ", tableName))
	if err != nil {
		return err
	}
	defer stmnt.Close()

	_, err = stmnt.Exec(newValue, property)
	return err
}

func (fi *Invocation) SetupDatabaseBackup(appBinding *appCatalog.AppBinding, repo *v1alpha1.Repository, transformFuncs ...func(bc *v1beta1.BackupConfiguration)) (*v1beta1.BackupConfiguration, error) {
	// Generate desired BackupConfiguration definition
	backupConfig := fi.GetBackupConfiguration(repo.Name, func(bc *v1beta1.BackupConfiguration) {
		bc.Spec.Target = &v1beta1.BackupTarget{
			Ref: GetTargetRef(appBinding.Name, apis.KindAppBinding),
		}
		bc.Spec.Task.Name = MySQLBackupTask
	})

	// transformFuncs provides a array of functions that made test specific change on the BackupConfiguration
	// apply these test specific changes
	for _, fn := range transformFuncs {
		fn(backupConfig)
	}

	By("Creating BackupConfiguration: " + backupConfig.Name)
	createdBC, err := fi.StashClient.StashV1beta1().BackupConfigurations(backupConfig.Namespace).Create(backupConfig)
	fi.AppendToCleanupList(createdBC)

	By("Verifying that backup triggering CronJob has been created")
	fi.EventuallyCronJobCreated(backupConfig.ObjectMeta).Should(BeTrue())

	return createdBC, err
}

func (fi *Invocation) SetupDatabaseRestore(appBinding *appCatalog.AppBinding, repo *v1alpha1.Repository, transformFuncs ...func(restore *v1beta1.RestoreSession)) (*v1beta1.RestoreSession, error) {
	// Generate desired RestoreSession definition
	By("Creating RestoreSession")
	restoreSession := fi.GetRestoreSession(repo.Name, func(restore *v1beta1.RestoreSession) {
		restore.Spec.Target = &v1beta1.RestoreTarget{
			Ref: GetTargetRef(appBinding.Name, apis.KindAppBinding),
		}
		restore.Spec.Rules = []v1beta1.Rule{
			{
				Snapshots: []string{"latest"},
			},
		}
		restore.Spec.Task.Name = MySQLRestoreTask
	})

	// transformFuncs provides a array of functions that made test specific change on the RestoreSession
	// apply these test specific changes.
	for _, fn := range transformFuncs {
		fn(restoreSession)
	}

	err := fi.CreateRestoreSession(restoreSession)
	fi.AppendToCleanupList(restoreSession)

	By("Waiting for restore process to complete")
	fi.EventuallyRestoreProcessCompleted(restoreSession.ObjectMeta).Should(BeTrue())

	return restoreSession, err
}

func (f *Framework) EnsureMySQLAddon() error {
	image := docker.Docker{
		Image:    "stash-mysql",
		Registry: f.DockerRegistry,
		Tag:      "8.0.14",
	}

	// create MySQL backup Function
	backupFunc := mysqlBackupFunction(image)
	_, err := f.StashClient.StashV1beta1().Functions().Create(backupFunc)
	if err != nil {
		return err
	}

	// create MySQL restore function
	restoreFunc := mysqlRestoreFunction(image)
	_, err = f.StashClient.StashV1beta1().Functions().Create(restoreFunc)
	if err != nil {
		return err
	}

	// create MySQL backup Task
	backupTask := mysqlBackupTask()
	_, err = f.StashClient.StashV1beta1().Tasks().Create(backupTask)
	if err != nil {
		return err
	}

	// create MySQL restore Task
	restoreTask := mysqlRestoreTask()
	_, err = f.StashClient.StashV1beta1().Tasks().Create(restoreTask)
	if err != nil {
		return err
	}

	return nil
}

func (f *Framework) EnsureMySQLAddonDeleted() error {
	// delete MySQL backup Function
	err := f.StashClient.StashV1beta1().Functions().Delete(MySQLBackupFunction, deleteInBackground())
	if err != nil {
		return err
	}

	// delete MySQL restore Function
	err = f.StashClient.StashV1beta1().Functions().Delete(MySQLRestoreFunction, deleteInBackground())
	if err != nil {
		return err
	}

	// delete MySQL backup Task
	err = f.StashClient.StashV1beta1().Tasks().Delete(MySQLBackupTask, deleteInBackground())
	if err != nil {
		return err
	}

	// delete MySQL restore Task
	err = f.StashClient.StashV1beta1().Tasks().Delete(MySQLRestoreTask, deleteInBackground())
	if err != nil {
		return err
	}
	return nil
}

func (fi *Invocation) MySQLAddonInstalled() bool {
	_, err := fi.StashClient.StashV1beta1().Functions().Get(MySQLBackupFunction, metav1.GetOptions{})
	if err != nil {
		return false
	}

	_, err = fi.StashClient.StashV1beta1().Functions().Get(MySQLRestoreFunction, metav1.GetOptions{})
	if err != nil {
		return false
	}

	_, err = fi.StashClient.StashV1beta1().Tasks().Get(MySQLBackupTask, metav1.GetOptions{})
	if err != nil {
		return false
	}

	_, err = fi.StashClient.StashV1beta1().Tasks().Get(MySQLRestoreTask, metav1.GetOptions{})

	return err == nil
}

func mysqlBackupFunction(image docker.Docker) *v1beta1.Function {
	return &v1beta1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Name: MySQLBackupFunction,
		},
		Spec: v1beta1.FunctionSpec{
			Image: image.ToContainerImage(),
			Args: []string{
				"backup-mysql",
				// setup information
				"--provider=${REPOSITORY_PROVIDER:=}",
				"--bucket=${REPOSITORY_BUCKET:=}",
				"--endpoint=${REPOSITORY_ENDPOINT:=}",
				"--path=${REPOSITORY_PREFIX:=}",
				"--secret-dir=/etc/repository/secret",
				"--scratch-dir=/tmp",
				"--enable-cache=${ENABLE_CACHE:=true}",
				"--max-connections=${MAX_CONNECTIONS:=0}",
				"--hostname=${HOSTNAME:=}",
				"--mysql-args=${myArgs:=--all-databases}",
				// target information
				"--appbinding=${TARGET_NAME:=}",
				"--namespace=${NAMESPACE:=default}",
				// cleanup information
				"--retention-keep-last=${RETENTION_KEEP_LAST:=0}",
				"--retention-keep-hourly=${RETENTION_KEEP_HOURLY:=0}",
				"--retention-keep-daily=${RETENTION_KEEP_DAILY:=0}",
				"--retention-keep-weekly=${RETENTION_KEEP_WEEKLY:=0}",
				"--retention-keep-monthly=${RETENTION_KEEP_MONTHLY:=0}",
				"--retention-keep-yearly=${RETENTION_KEEP_YEARLY:=0}",
				"--retention-keep-tags=${RETENTION_KEEP_TAGS:=}",
				"--retention-prune=${RETENTION_PRUNE:=false}",
				"--retention-dry-run=${RETENTION_DRY_RUN:=false}",
				// output information
				"--output-dir=${outputDir:=}",
			},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      "${secretVolume}",
					MountPath: "/etc/repository/secret",
				},
			},
		},
	}
}

func mysqlRestoreFunction(image docker.Docker) *v1beta1.Function {
	return &v1beta1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Name: MySQLRestoreFunction,
		},
		Spec: v1beta1.FunctionSpec{
			Image: image.ToContainerImage(),
			Args: []string{
				"restore-mysql",
				// setup information
				"--provider=${REPOSITORY_PROVIDER:=}",
				"--bucket=${REPOSITORY_BUCKET:=}",
				"--endpoint=${REPOSITORY_ENDPOINT:=}",
				"--path=${REPOSITORY_PREFIX:=}",
				"--secret-dir=/etc/repository/secret",
				"--scratch-dir=/tmp",
				"--enable-cache=${ENABLE_CACHE:=true}",
				"--max-connections=${MAX_CONNECTIONS:=0}",
				"--hostname=${HOSTNAME:=}",
				"--source-hostname=${SOURCE_HOSTNAME:=}",
				"--mysql-args=${myArgs:=}",
				// target information
				"--appbinding=${TARGET_NAME:=}",
				"--namespace=${NAMESPACE:=default}",
				"--snapshot=${RESTORE_SNAPSHOTS:=}",
				// output information
				"--output-dir=${outputDir:=}",
			},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      "${secretVolume}",
					MountPath: "/etc/repository/secret",
				},
			},
		},
	}
}

func mysqlBackupTask() *v1beta1.Task {
	return &v1beta1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name: MySQLBackupTask,
		},
		Spec: v1beta1.TaskSpec{
			Steps: []v1beta1.FunctionRef{
				{
					Name: MySQLBackupFunction,
					Params: []v1beta1.Param{
						{
							Name:  "outputDir",
							Value: "/tmp/output",
						},
						{
							Name:  "secretVolume",
							Value: "secret-volume",
						},
					},
				},
				{
					Name: "update-status",
					Params: []v1beta1.Param{
						{
							Name:  "outputDir",
							Value: "/tmp/output",
						},
					},
				},
			},
			Volumes: []core.Volume{
				{
					Name: "secret-volume",
					VolumeSource: core.VolumeSource{
						Secret: &core.SecretVolumeSource{
							SecretName: "${REPOSITORY_SECRET_NAME}",
						},
					},
				},
			},
		},
	}
}

func mysqlRestoreTask() *v1beta1.Task {
	return &v1beta1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name: MySQLRestoreTask,
		},
		Spec: v1beta1.TaskSpec{
			Steps: []v1beta1.FunctionRef{
				{
					Name: MySQLRestoreFunction,
					Params: []v1beta1.Param{
						{
							Name:  "outputDir",
							Value: "/tmp/output",
						},
						{
							Name:  "secretVolume",
							Value: "secret-volume",
						},
					},
				},
				{
					Name: "update-status",
					Params: []v1beta1.Param{
						{
							Name:  "outputDir",
							Value: "/tmp/output",
						},
					},
				},
			},
			Volumes: []core.Volume{
				{
					Name: "secret-volume",
					VolumeSource: core.VolumeSource{
						Secret: &core.SecretVolumeSource{
							SecretName: "${REPOSITORY_SECRET_NAME}",
						},
					},
				},
			},
		},
	}
}
