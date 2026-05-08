package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	queueti "github.com/Joessst-Dev/queue-ti/clients/go-client"
)

// SeedFile is the top-level structure of the JSON seed file.
type SeedFile struct {
	TopicConfigs   []queueti.TopicConfig `json:"topic_configs"`
	TopicSchemas   []TopicSchemaEntry    `json:"topic_schemas"`
	ConsumerGroups []ConsumerGroupEntry  `json:"consumer_groups"`
}

type TopicSchemaEntry struct {
	Topic  string `json:"topic"`
	Schema string `json:"schema"`
}

type ConsumerGroupEntry struct {
	Topic  string   `json:"topic"`
	Groups []string `json:"groups"`
}

// Seeder applies a SeedFile against a queue-ti admin API.
type Seeder struct {
	admin  *queueti.AdminClient
	dryRun bool
	log    *slog.Logger
}

func newSeeder(admin *queueti.AdminClient, dryRun bool, log *slog.Logger) *Seeder {
	return &Seeder{admin: admin, dryRun: dryRun, log: log}
}

// Apply upserts all resources declared in f.
// Topic configs and schemas are always PUT (idempotent).
// Consumer groups are registered only when not already present.
func (s *Seeder) Apply(ctx context.Context, f *SeedFile) error {
	for _, cfg := range f.TopicConfigs {
		if s.dryRun {
			s.log.Info("dry-run: would upsert topic config", "topic", cfg.Topic)
			continue
		}
		if _, err := s.admin.UpsertTopicConfig(ctx, cfg.Topic, cfg); err != nil {
			return fmt.Errorf("topic config %q: %w", cfg.Topic, err)
		}
		s.log.Info("upserted topic config", "topic", cfg.Topic)
	}

	for _, schema := range f.TopicSchemas {
		if s.dryRun {
			s.log.Info("dry-run: would upsert topic schema", "topic", schema.Topic)
			continue
		}
		if _, err := s.admin.UpsertTopicSchema(ctx, schema.Topic, schema.Schema); err != nil {
			return fmt.Errorf("topic schema %q: %w", schema.Topic, err)
		}
		s.log.Info("upserted topic schema", "topic", schema.Topic)
	}

	for _, entry := range f.ConsumerGroups {
		if s.dryRun {
			for _, group := range entry.Groups {
				s.log.Info("dry-run: would register consumer group (if not exists)", "topic", entry.Topic, "group", group)
			}
			continue
		}
		existing, err := s.admin.ListConsumerGroups(ctx, entry.Topic)
		if err != nil {
			return fmt.Errorf("list consumer groups for topic %q: %w", entry.Topic, err)
		}
		for _, group := range entry.Groups {
			if slices.Contains(existing, group) {
				s.log.Info("consumer group already exists, skipping", "topic", entry.Topic, "group", group)
				continue
			}
			if err := s.admin.RegisterConsumerGroup(ctx, entry.Topic, group); err != nil && !errors.Is(err, queueti.ErrConflict) {
				return fmt.Errorf("register consumer group %q on topic %q: %w", group, entry.Topic, err)
			}
			s.log.Info("registered consumer group", "topic", entry.Topic, "group", group)
		}
	}

	return nil
}
