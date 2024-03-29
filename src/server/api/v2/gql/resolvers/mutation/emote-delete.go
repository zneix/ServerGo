package mutation_resolvers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/SevenTV/ServerGo/src/aws"
	"github.com/SevenTV/ServerGo/src/configure"
	"github.com/SevenTV/ServerGo/src/discord"
	"github.com/SevenTV/ServerGo/src/mongo"
	"github.com/SevenTV/ServerGo/src/mongo/datastructure"
	"github.com/SevenTV/ServerGo/src/server/api/v2/gql/resolvers"
	"github.com/SevenTV/ServerGo/src/utils"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (*MutationResolver) DeleteEmote(ctx context.Context, args struct {
	ID     string
	Reason string
}) (*bool, error) {
	if args.Reason == "" {
		return nil, resolvers.ErrNoReason
	}

	var success bool

	usr, ok := ctx.Value(utils.UserKey).(*datastructure.User)
	if !ok {
		return nil, resolvers.ErrLoginRequired
	}

	id, err := primitive.ObjectIDFromHex(args.ID)
	if err != nil {
		return nil, resolvers.ErrUnknownEmote
	}

	res := mongo.Database.Collection("emotes").FindOne(ctx, bson.M{
		"_id": id,
		"status": bson.M{
			"$ne": datastructure.EmoteStatusDeleted,
		},
	})

	emote := &datastructure.Emote{}

	err = res.Err()

	if err == nil {
		err = res.Decode(emote)
	}
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, resolvers.ErrUnknownEmote
		}
		log.Errorf("mongo, err=%v", err)
		return nil, resolvers.ErrInternalServer
	}

	if !usr.HasPermission(datastructure.RolePermissionEmoteEditAll) {
		if emote.OwnerID.Hex() != usr.ID.Hex() {
			if err := mongo.Database.Collection("users").FindOne(ctx, bson.M{
				"_id":     emote.OwnerID,
				"editors": usr.ID,
			}).Err(); err != nil {
				if err == mongo.ErrNoDocuments {
					return nil, resolvers.ErrAccessDenied
				}
				log.Errorf("mongo, err=%v", err)
				return nil, resolvers.ErrInternalServer
			}
		}
	}

	_, err = mongo.Database.Collection("emotes").UpdateOne(ctx, bson.M{
		"_id": id,
	}, bson.M{
		"$set": bson.M{
			"status":             datastructure.EmoteStatusDeleted,
			"last_modified_date": time.Now(),
		},
	})

	if err != nil {
		log.Errorf("mongo, err=%v", err)
		return nil, resolvers.ErrInternalServer
	}

	_, err = mongo.Database.Collection("audit").InsertOne(ctx, &datastructure.AuditLog{
		Type:      datastructure.AuditLogTypeEmoteDelete,
		CreatedBy: usr.ID,
		Target:    &datastructure.Target{ID: &id, Type: "emotes"},
		Changes: []*datastructure.AuditLogChange{
			{Key: "status", OldValue: emote.Status, NewValue: datastructure.EmoteStatusDeleted},
		},
		Reason: &args.Reason,
	})
	if err != nil {
		log.Errorf("mongo, err=%v", err)
	}

	wg := &sync.WaitGroup{}
	wg.Add(4)

	for i := 1; i <= 4; i++ {
		go func(i int) {
			defer wg.Done()
			obj := fmt.Sprintf("emote/%s", emote.ID.Hex())
			err := aws.Expire(configure.Config.GetString("aws_cdn_bucket"), obj, i)
			if err != nil {
				log.Errorf("aws, err=%v, obj=%s", err, obj)
			}
		}(i)
	}

	_, err = mongo.Database.Collection("users").UpdateMany(ctx, bson.M{
		"emotes": id,
	}, bson.M{
		"$pull": bson.M{
			"emotes": id,
		},
	})
	if err != nil {
		log.Errorf("mongo, err=%v", err)
	}

	wg.Wait()

	go discord.SendEmoteDelete(*emote, *usr, args.Reason)
	success = true
	return &success, nil
}
