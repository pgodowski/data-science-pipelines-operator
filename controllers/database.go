/*

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

package controllers

import (
	"context"
	"database/sql"
	b64 "encoding/base64"
	"fmt"

	"time"

	"errors"

	_ "github.com/go-sql-driver/mysql"
	dspav1alpha1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1alpha1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
)

const dbSecret = "mariadb/secret.yaml.tmpl"

var mariadbTemplates = []string{
	"mariadb/deployment.yaml.tmpl",
	"mariadb/pvc.yaml.tmpl",
	"mariadb/service.yaml.tmpl",
	"mariadb/mariadb-sa.yaml.tmpl",
}

// extract to var for mocking in testing
var ConnectAndQueryDatabase = func(host, port, username, password, dbname string, dbConnectionTimeout time.Duration) (bool, error) {
	// Create a context with a timeout of 1 second
	ctx, cancel := context.WithTimeout(context.Background(), dbConnectionTimeout)
	defer cancel()

	connectionString := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", username, password, host, port, dbname)
	db, err := sql.Open("mysql", connectionString)
	if err != nil {
		return false, err
	}
	defer db.Close()

	testStatement := "SELECT 1;"
	_, err = db.QueryContext(ctx, testStatement)
	return err == nil, nil
}

func (r *DSPAReconciler) isDatabaseAccessible(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) (bool, error) {
	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)

	if params.DatabaseHealthCheckDisabled(dsp) {
		infoMessage := "Database health check disabled, assuming database is available and ready."
		log.V(1).Info(infoMessage)
		return true, nil
	}

	log.Info("Performing Database Health Check")
	databaseSpecified := dsp.Spec.Database != nil
	usingExternalDB := params.UsingExternalDB(dsp)
	usingMariaDB := !databaseSpecified || dsp.Spec.Database.MariaDB != nil
	if !usingMariaDB && !usingExternalDB {
		errorMessage := "Could not connect to Database: Unsupported Type"
		log.Error(nil, errorMessage)
		return false, errors.New(errorMessage)
	}

	decodePass, _ := b64.StdEncoding.DecodeString(params.DBConnection.Password)
	dbConnectionTimeout := config.GetDurationConfigWithDefault(config.DBConnectionTimeoutConfigName, config.DefaultDBConnectionTimeout)

	log.V(1).Info(fmt.Sprintf("Database Heath Check connection timeout: %s", dbConnectionTimeout))

	dbHealthCheckPassed, err := ConnectAndQueryDatabase(params.DBConnection.Host,
		params.DBConnection.Port,
		params.DBConnection.Username,
		string(decodePass),
		params.DBConnection.DBName,
		dbConnectionTimeout)

	if err != nil {
		log.Info("Unable to connect to Database")
	} else {
		log.Info("Database Health Check Successful")
	}

	return dbHealthCheckPassed, err
}

func (r *DSPAReconciler) ReconcileDatabase(ctx context.Context, dsp *dspav1alpha1.DataSciencePipelinesApplication,
	params *DSPAParams) error {

	log := r.Log.WithValues("namespace", dsp.Namespace).WithValues("dspa_name", dsp.Name)
	databaseSpecified := dsp.Spec.Database != nil
	// DB field can be specified as an empty obj, confirm that subfields are also specified
	// By default if Database is empty, we deploy mariadb
	externalDBSpecified := params.UsingExternalDB(dsp)
	mariaDBSpecified := dsp.Spec.Database.MariaDB != nil
	defaultDBRequired := !databaseSpecified || (!externalDBSpecified && !mariaDBSpecified)

	deployMariaDB := mariaDBSpecified && dsp.Spec.Database.MariaDB.Deploy
	// Default DB is currently MariaDB as well, but storing these bools seperately in case that changes
	deployDefaultDB := !databaseSpecified || defaultDBRequired

	externalDBCredentialsProvided := externalDBSpecified && (dsp.Spec.Database.ExternalDB.PasswordSecret != nil)
	mariaDBCredentialsProvided := mariaDBSpecified && (dsp.Spec.Database.MariaDB.PasswordSecret != nil)
	databaseCredentialsProvided := externalDBCredentialsProvided || mariaDBCredentialsProvided

	// If external db is specified, it takes precedence
	if externalDBSpecified {
		log.Info("Using externalDB, bypassing database deployment.")
	} else if deployMariaDB || deployDefaultDB {
		if !databaseCredentialsProvided {
			err := r.Apply(dsp, params, dbSecret)
			if err != nil {
				return err
			}
		}
		log.Info("Applying mariaDB resources.")
		for _, template := range mariadbTemplates {
			err := r.Apply(dsp, params, template)
			if err != nil {
				return err
			}
		}
		// If no database was not specified, deploy mariaDB by default.
		// Update the CR with the state of mariaDB to accurately portray
		// desired state.
		if !databaseSpecified {
			dsp.Spec.Database = &dspav1alpha1.Database{}
		}
		if !databaseSpecified || defaultDBRequired {
			dsp.Spec.Database.MariaDB = params.MariaDB.DeepCopy()
			dsp.Spec.Database.MariaDB.Deploy = true
			if err := r.Update(ctx, dsp); err != nil {
				return err
			}
		}
	} else {
		log.Info("No externalDB detected, and mariaDB disabled. " +
			"skipping Application of DB Resources")
		return nil
	}
	log.Info("Finished applying Database Resources")

	return nil
}
