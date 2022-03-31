// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"context"
	"time"

	"github.com/omec-project/upf-epc/pfcpiface/metrics"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoDBStore struct {
	client *mongo.Client
	db     string
	coll   string
}

type PFCPSessionDoc struct {
	Fseid      uint64
	LocalSEID  uint64
	RemoteSEID uint64
	Metrics    *metrics.Session
	PacketForwardingRules
}

func NewMongoDBStore() *MongoDBStore {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	defer cancel()

	serverAPIOptions := options.ServerAPI(options.ServerAPIVersion1)
	clientOptions := options.Client().
		ApplyURI("mongodb+srv://onf:opennetworking@cluster0.ld6zn.mongodb.net/myFirstDatabase?retryWrites=true&w=majority").
		// ApplyURI("mongodb://mongo:5001").
		SetServerAPIOptions(serverAPIOptions)
	client, err := mongo.Connect(ctx, clientOptions)

	if err != nil {
		log.Fatal(err)
	}

	database := client.Database("sessionsDatabase")
	database.RunCommand(context.TODO(), bson.M{"create": "sessions"})

	return &MongoDBStore{client, "sessionsDatabase", "sessions"}
}

func (i *MongoDBStore) GetAllSessions() []PFCPSession {
	sessions := make([]PFCPSession, 0)

	cur, err := i.client.Database(i.db).Collection(i.coll).Find(context.TODO(), bson.M{})

	if err != nil {
		log.Fatal(err)
	}

	for cur.Next(context.TODO()) {
		var elem PFCPSession
		err := cur.Decode(&elem)

		if err != nil {
			log.Fatal(err)
		}

		sessions = append(sessions, elem)
	}

	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}

	cur.Close(context.TODO())

	log.WithFields(log.Fields{
		"sessions": sessions,
	}).Trace("Got all PFCP sessions from local store")

	return sessions
}

func (i *MongoDBStore) PutSession(session PFCPSession) error {
	if session.localSEID == 0 {
		return ErrInvalidArgument("session.localSEID", session.localSEID)
	}

	_, err := i.client.Database(i.db).Collection(i.coll).InsertOne(context.TODO(), bson.M{"fseid": int64(session.localSEID), "localSEID": int64(session.localSEID),
		"remoteSEID":            int64(session.remoteSEID),
		"metrics":               session.metrics,
		"PacketForwardingRules": bson.M{"fars": session.PacketForwardingRules.fars, "pdrs": session.PacketForwardingRules.pdrs, "qers": session.PacketForwardingRules.qers},
	})

	if err != nil {
		log.Fatal(err)
	}

	log.WithFields(log.Fields{
		"session": session,
	}).Trace("Saved PFCP sessions to local store")

	return nil
}

func (i *MongoDBStore) DeleteSession(fseid uint64) error {
	_, err := i.client.Database(i.db).Collection(i.coll).DeleteOne(context.TODO(), bson.M{"fseid": fseid})

	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"F-SEID": fseid,
	}).Trace("PFCP session removed from local store")

	return nil
}

func (i *MongoDBStore) DeleteAllSessions() bool {
	_, err := i.client.Database(i.db).Collection(i.coll).DeleteMany(context.TODO(), bson.M{})

	if err != nil {
		log.Fatal(err)
	}

	log.Trace("All PFCP sessions removed from local store")

	return true
}

func (i *MongoDBStore) GetSession(fseid uint64) (PFCPSession, bool) {
	filter := bson.M{"fseid": int64(fseid)}

	var session PFCPSessionDoc

	err := i.client.Database(i.db).Collection(i.coll).FindOne(context.TODO(), filter).Decode(&session)

	if err != nil {
		log.Error(err)
		return PFCPSession{}, false
	}

	log.WithFields(log.Fields{
		"session": session,
	}).Trace("Got PFCP session from local store")

	return PFCPSession{localSEID: session.LocalSEID, remoteSEID: session.RemoteSEID, metrics: session.Metrics, PacketForwardingRules: session.PacketForwardingRules}, true
}
