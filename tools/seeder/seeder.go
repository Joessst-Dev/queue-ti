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

// validate returns an error if any entry has an empty topic field.
func (f *SeedFile) validate() error {
	for i, cfg := range f.TopicConfigs {
		if cfg.Topic == "" {
			return fmt.Errorf("topic_configs[%d]: topic must not be empty", i)
		}
	}
	for i, s := range f.TopicSchemas {
		if s.Topic == "" {
			return fmt.Errorf("topic_schemas[%d]: topic must not be empty", i)
		}
	}
	for i, cg := range f.ConsumerGroups {
		if cg.Topic == "" {
			return fmt.Errorf("consumer_groups[%d]: topic must not be empty", i)
		}
	}
	return nil
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
func (s *Seeder) Apply(ctx context.Context, f *SeedFile) error {
	if err := s.applyTopicConfigs(ctx, f.TopicConfigs); err != nil {
		return err
	}
	if err := s.applyTopicSchemas(ctx, f.TopicSchemas); err != nil {
		return err
	}
	return s.applyConsumerGroups(ctx, f.ConsumerGroups)
}

func (s *Seeder) applyTopicConfigs(ctx context.Context, configs []queueti.TopicConfig) error {
	for _, cfg := range configs {
		if s.dryRun {
			s.log.Info("dry-run: would upsert topic config", "topic", cfg.Topic)
			continue
		}
		if _, err := s.admin.UpsertTopicConfig(ctx, cfg.Topic, cfg); err != nil {
			return fmt.Errorf("topic config %q: %w", cfg.Topic, err)
		}
		s.log.Info("upserted topic config", "topic", cfg.Topic)
	}
	return nil
}

func (s *Seeder) applyTopicSchemas(ctx context.Context, schemas []TopicSchemaEntry) error {
	for _, schema := range schemas {
		if s.dryRun {
			s.log.Info("dry-run: would upsert topic schema", "topic", schema.Topic)
			continue
		}
		if _, err := s.admin.UpsertTopicSchema(ctx, schema.Topic, schema.Schema); err != nil {
			return fmt.Errorf("topic schema %q: %w", schema.Topic, err)
		}
		s.log.Info("upserted topic schema", "topic", schema.Topic)
	}
	return nil
}

func (s *Seeder) applyConsumerGroups(ctx context.Context, entries []ConsumerGroupEntry) error {
	for _, entry := range entries {
		if len(entry.Groups) == 0 {
			s.log.Warn("consumer group entry has no groups, skipping", "topic", entry.Topic)
			continue
		}
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
