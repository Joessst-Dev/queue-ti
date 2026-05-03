import { Component, inject, signal, ChangeDetectionStrategy, OnInit } from '@angular/core';
import { DatePipe } from '@angular/common';
import { QueueService, TopicSchema } from '../services/queue.service';
import { getErrorMessage } from '../utils/error';
import { inputValue } from '../utils/dom';

@Component({
  selector: 'app-topic-schema-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DatePipe],
  template: `
    <section class="bg-white shadow rounded-lg">
      <div class="px-6 py-4 border-b border-gray-200">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="flex items-center gap-2 text-lg font-semibold text-gray-900">
              <svg
                class="w-5 h-5 text-gray-400"
                fill="none"
                viewBox="0 0 24 24"
                stroke-width="1.5"
                stroke="currentColor"
                aria-hidden="true"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M17.25 6.75 22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3-4.5 16.5"
                />
              </svg>
              Avro Schema Validation
            </h2>
            <p class="mt-1 text-sm text-gray-500">
              Register an Avro schema to validate message payloads before enqueue.
            </p>
          </div>
          <button
            type="button"
            (click)="onAddNew()"
            [disabled]="addingNew()"
            class="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-indigo-600 border border-indigo-300 rounded-md hover:bg-indigo-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
            </svg>
            Add Schema
          </button>
        </div>
      </div>

      <div class="px-6 py-4">
        @if (error()) {
          <div class="mb-4 p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm" role="alert">
            {{ error() }}
          </div>
        }

        @if (loading()) {
          <p class="text-sm text-gray-500">Loading…</p>
        } @else if (schemas().length === 0 && !addingNew()) {
          <p class="text-sm text-gray-500">No schemas registered.</p>
        } @else {
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Topic</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Version</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Updated</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100">
                @if (addingNew()) {
                  <tr class="bg-indigo-50">
                    <td class="px-3 py-2">
                      <input
                        type="text"
                        [value]="newTopic()"
                        (input)="newTopic.set(inputValue($event))"
                        placeholder="topic name"
                        aria-label="New topic name"
                        class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                      />
                    </td>
                    <td class="px-3 py-2 text-gray-400">—</td>
                    <td class="px-3 py-2 text-gray-400">—</td>
                    <td class="px-3 py-2">
                      <textarea
                        [value]="newSchemaJson()"
                        (input)="newSchemaJson.set(inputValue($event))"
                        rows="8"
                        placeholder='{"type":"record","name":"...","fields":[]}'
                        aria-label="New schema JSON"
                        class="w-full px-3 py-1.5 text-sm font-mono border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                      ></textarea>
                      <div class="flex items-center gap-2 mt-2">
                        <button
                          type="button"
                          (click)="onSaveNew()"
                          class="px-3 py-1 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer"
                        >
                          Save
                        </button>
                        <button
                          type="button"
                          (click)="onCancelNew()"
                          class="px-3 py-1 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
                        >
                          Cancel
                        </button>
                      </div>
                    </td>
                  </tr>
                }
                @for (schema of schemas(); track schema.topic) {
                  @if (editingTopic() === schema.topic) {
                    <tr class="bg-indigo-50">
                      <td class="px-3 py-2 font-medium text-gray-900">{{ schema.topic }}</td>
                      <td class="px-3 py-2 text-gray-700">{{ schema.version }}</td>
                      <td class="px-3 py-2 text-gray-500">{{ schema.updated_at | date: 'short' }}</td>
                      <td class="px-3 py-2">
                        <textarea
                          [value]="editForm().schema_json"
                          (input)="patchEditForm(inputValue($event))"
                          rows="8"
                          [attr.aria-label]="'Schema JSON for ' + schema.topic"
                          class="w-full px-3 py-1.5 text-sm font-mono border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                        ></textarea>
                        <div class="flex items-center gap-2 mt-2">
                          <button
                            type="button"
                            (click)="onSave(schema.topic)"
                            class="px-3 py-1 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer"
                          >
                            Save
                          </button>
                          <button
                            type="button"
                            (click)="onCancelEdit()"
                            class="px-3 py-1 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
                          >
                            Cancel
                          </button>
                        </div>
                      </td>
                    </tr>
                  } @else {
                    <tr class="hover:bg-gray-50">
                      <td class="px-3 py-2 font-medium text-gray-900">{{ schema.topic }}</td>
                      <td class="px-3 py-2 text-gray-700">{{ schema.version }}</td>
                      <td class="px-3 py-2 text-gray-500">{{ schema.updated_at | date: 'short' }}</td>
                      <td class="px-3 py-2 flex items-center gap-2">
                        <button
                          type="button"
                          (click)="onEdit(schema)"
                          [attr.aria-label]="'Edit schema for ' + schema.topic"
                          class="px-3 py-1 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          (click)="onDelete(schema.topic)"
                          [attr.aria-label]="'Delete schema for ' + schema.topic"
                          class="px-2 py-1 text-lg text-gray-400 hover:text-red-600 focus:outline-none cursor-pointer"
                        >
                          &times;
                        </button>
                      </td>
                    </tr>
                  }
                }
              </tbody>
            </table>
          </div>
        }
      </div>
    </section>
  `,
})
export class TopicSchemaSection implements OnInit {
  private readonly queue = inject(QueueService);

  readonly schemas = signal<TopicSchema[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');
  readonly editingTopic = signal<string | null>(null);
  readonly editForm = signal<{ schema_json: string }>({ schema_json: '' });
  readonly addingNew = signal(false);

  readonly newTopic = signal('');
  readonly newSchemaJson = signal('');

  protected readonly inputValue = inputValue;

  ngOnInit(): void {
    this.loadSchemas();
  }

  loadSchemas(): void {
    this.loading.set(true);
    this.error.set('');
    this.queue.getTopicSchemas().subscribe({
      next: (schemas) => {
        this.schemas.set(schemas);
        this.loading.set(false);
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to load schemas'));
        this.loading.set(false);
      },
    });
  }

  onEdit(schema: TopicSchema): void {
    this.editForm.set({ schema_json: schema.schema_json });
    this.editingTopic.set(schema.topic);
  }

  onCancelEdit(): void {
    this.editingTopic.set(null);
  }

  patchEditForm(value: string): void {
    this.editForm.set({ schema_json: value });
  }

  onSave(topic: string): void {
    this.queue.upsertTopicSchema(topic, this.editForm().schema_json).subscribe({
      next: (updated) => {
        this.schemas.update((list) =>
          list.map((s) => (s.topic === topic ? updated : s)),
        );
        this.editingTopic.set(null);
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to save schema'));
      },
    });
  }

  onDelete(topic: string): void {
    this.queue.deleteTopicSchema(topic).subscribe({
      next: () => {
        this.schemas.update((list) => list.filter((s) => s.topic !== topic));
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to delete schema'));
      },
    });
  }

  onAddNew(): void {
    this.newTopic.set('');
    this.newSchemaJson.set('');
    this.addingNew.set(true);
  }

  onCancelNew(): void {
    this.addingNew.set(false);
  }

  onSaveNew(): void {
    const topic = this.newTopic().trim();
    if (!topic) {
      this.error.set('Topic name is required');
      return;
    }
    this.queue.upsertTopicSchema(topic, this.newSchemaJson()).subscribe({
      next: (created) => {
        this.schemas.update((list) => [...list, created]);
        this.addingNew.set(false);
        this.newTopic.set('');
        this.newSchemaJson.set('');
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to save schema'));
      },
    });
  }
}
