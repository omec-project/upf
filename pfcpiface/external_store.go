// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoBDStore struct {
	coll *mongo.Collection
}

// type PFCPSessionDocument struct {
// 	fseid       uint64
// 	PFCPSession PFCPSession
// }

func NewMongoBDStore() *MongoBDStore {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://datastore:27017/"))

	if err != nil {
		log.Fatal(err)
	}

	sessionsCollection := client.Database("sessionsDatabase").Collection("sessions")

	return &MongoBDStore{sessionsCollection}
}

func (i *MongoBDStore) GetAllSessions() []PFCPSession {
	sessions := make([]PFCPSession, 0)

	opts := options.Find().SetProjection(bson.D{{"fseid", 0}, {"session", 1}})
	cur, err := i.coll.Find(context.TODO(), bson.D{}, opts)

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

func (i *MongoBDStore) PutSession(session PFCPSession) error {
	if session.localSEID == 0 {
		return ErrInvalidArgument("session.localSEID", session.localSEID)
	}

	doc := bson.D{{"fseid", session.localSEID}, {"session", session}}
	_, err := i.coll.InsertOne(context.TODO(), doc)

	if err != nil {
		log.Fatal(err)
	}

	log.WithFields(log.Fields{
		"session": session,
	}).Trace("Saved PFCP sessions to local store")

	return nil
}

func (i *MongoBDStore) DeleteSession(fseid uint64) error {
	_, err := i.coll.DeleteOne(context.TODO(), bson.D{{"fseid", fseid}})
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"F-SEID": fseid,
	}).Trace("PFCP session removed from local store")

	return nil
}

func (i *MongoBDStore) DeleteAllSessions() bool {
	_, err := i.coll.DeleteMany(context.TODO(), bson.D{})

	if err != nil {
		log.Fatal(err)
	}

	log.Trace("All PFCP sessions removed from local store")

	return true
}

func (i *MongoBDStore) GetSession(fseid uint64) (PFCPSession, bool) {
	filter := bson.D{{"fseid", fseid}}

	var session PFCPSession

	err := i.coll.FindOne(context.TODO(), filter).Decode(&session)

	if err != nil {
		log.Fatal(err)
	}

	log.WithFields(log.Fields{
		"session": session,
	}).Trace("Got PFCP session from local store")

	return session, true
}
