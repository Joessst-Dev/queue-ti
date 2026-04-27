import {
  Component,
  input,
  output,
  signal,
  effect,
  inject,
  DestroyRef,
  ChangeDetectionStrategy,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Subject, debounceTime, switchMap, catchError, of } from 'rxjs';
import { FormField, form, schema, required } from '@angular/forms/signals';
import { EnqueueRequest, QueueService, TopicSchema } from '../services/queue.service';
import { generateAvroExample } from './avro-example';

interface EnqueueModel {
  topic: string;
  payload: string;
}

interface MetadataRowModel {
  key: string;
  value: string;
}

@Component({
  selector: 'app-enqueue-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormField],
  template: `
    <section class="bg-white shadow rounded-lg">
      <div class="px-6 py-4 border-b border-gray-200">
        <h2
          class="flex items-center gap-2 text-lg font-semibold text-gray-900"
        >
          <svg
            class="w-5 h-5 text-gray-400"
            fill="none"
            viewBox="0 0 24 24"
            stroke-width="1.5"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M12 9v6m3-3H9m12 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"
            />
          </svg>
          Enqueue Message
        </h2>
      </div>
      <div class="px-6 py-4">
        @if (success()) {
          <div
            class="mb-4 p-3 bg-green-50 border border-green-200 text-green-700 rounded text-sm"
          >
            Message enqueued successfully (ID: {{ success() }})
          </div>
        }
        @if (error()) {
          <div
            class="mb-4 p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm"
          >
            {{ error() }}
          </div>
        }

        <form (submit)="onSubmit($event)" class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label
                for="enqueue-topic"
                class="block text-sm font-medium text-gray-700 mb-1"
                >Topic</label
              >
              <input
                id="enqueue-topic"
                type="text"
                [formField]="enqueueForm.topic"
                (input)="onTopicInput($any($event.target).value)"
                class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
                placeholder="e.g. orders"
              />
            </div>
          </div>

          <div>
            <div class="flex items-center justify-between mb-1">
              <label
                for="enqueue-payload"
                class="block text-sm font-medium text-gray-700"
              >
                Payload
              </label>
              @if (topicSchema()) {
                <button type="button" (click)="fillExample()" class="text-sm text-indigo-600 hover:text-indigo-800 cursor-pointer">
                  Fill example
                </button>
              }
            </div>
            <textarea
              id="enqueue-payload"
              [formField]="enqueueForm.payload"
              rows="4"
              class="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 font-mono text-sm"
              placeholder='{"key": "value"}'
            ></textarea>
          </div>

          <!-- Metadata -->
          <div>
            <div class="flex items-center justify-between mb-2">
              <label
                for="add-metadata-field"
                class="block text-sm font-medium text-gray-700"
              >
                Metadata
              </label>
              <button
                id="add-metadata-field"
                type="button"
                (click)="addMetadataRow()"
                class="text-sm text-indigo-600 hover:text-indigo-800 cursor-pointer"
              >
                <svg
                  class="inline w-4 h-4 mr-0.5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M12 4.5v15m7.5-7.5h-15"
                  />
                </svg>
                Add field
              </button>
            </div>
            @for (row of metadataRows(); track $index) {
              <div class="flex gap-2 mb-2">
                <input
                  type="text"
                  [formField]="row.form.key"
                  [attr.aria-label]="'Metadata key ' + ($index + 1)"
                  placeholder="Key"
                  class="flex-1 px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                />
                <input
                  type="text"
                  [formField]="row.form.value"
                  [attr.aria-label]="'Metadata value ' + ($index + 1)"
                  placeholder="Value"
                  class="flex-1 px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                />
                <button
                  type="button"
                  (click)="removeMetadataRow($index)"
                  class="px-2 text-gray-400 hover:text-red-600 text-lg cursor-pointer"
                >
                  &times;
                </button>
              </div>
            }
          </div>

          <button
            type="submit"
            [disabled]="loading()"
            class="px-4 py-2 bg-indigo-600 text-white font-medium rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            @if (loading()) {
              <svg
                class="inline w-4 h-4 mr-1 animate-spin"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  class="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  stroke-width="4"
                ></circle>
                <path
                  class="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                ></path>
              </svg>
              Sending...
            } @else {
              <svg
                class="inline w-4 h-4 mr-1"
                fill="none"
                viewBox="0 0 24 24"
                stroke-width="1.5"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M6 12 3.269 3.125A59.769 59.769 0 0 1 21.485 12 59.768 59.768 0 0 1 3.27 20.875L5.999 12Zm0 0h7.5"
                />
              </svg>
              Enqueue
            }
          </button>
        </form>
      </div>
    </section>
  `,
})
export class EnqueueSection {
  readonly success = input<string>('');
  readonly error = input<string>('');
  readonly loading = input<boolean>(false);

  readonly enqueue = output<EnqueueRequest>();

  readonly enqueueModel = signal<EnqueueModel>({ topic: '', payload: '' });

  readonly enqueueForm = form(
    this.enqueueModel,
    schema<EnqueueModel>((root) => {
      required(root.topic);
      required(root.payload);
    }),
  );

  readonly metadataRows = signal<
    {
      model: ReturnType<typeof signal<MetadataRowModel>>;
      form: ReturnType<typeof form<MetadataRowModel>>;
    }[]
  >([]);

  private readonly queue = inject(QueueService);
  private readonly destroyRef = inject(DestroyRef);
  private readonly topicChange$ = new Subject<string>();
  readonly topicSchema = signal<TopicSchema | null>(null);

  constructor() {
    effect(() => {
      if (this.success()) {
        this.enqueueModel.set({ topic: '', payload: '' });
        this.metadataRows.set([]);
        this.topicSchema.set(null);
      }
    });

    this.topicChange$.pipe(
      debounceTime(300),
      switchMap((topic) => {
        if (!topic.trim()) return of(null);
        return this.queue.getTopicSchema(topic).pipe(catchError(() => of(null)));
      }),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe((schema) => this.topicSchema.set(schema));
  }

  addMetadataRow(): void {
    const model = signal<MetadataRowModel>({ key: '', value: '' });
    this.metadataRows.update((rows) => [...rows, { model, form: form(model) }]);
  }

  removeMetadataRow(index: number): void {
    this.metadataRows.update((rows) => rows.filter((_, i) => i !== index));
  }

  onTopicInput(value: string): void {
    this.topicChange$.next(value);
  }

  fillExample(): void {
    const schema = this.topicSchema();
    if (!schema) return;
    try {
      const example = generateAvroExample(JSON.parse(schema.schema_json));
      this.enqueueModel.update((m) => ({ ...m, payload: JSON.stringify(example, null, 2) }));
    } catch {
      // malformed schema_json — no-op
    }
  }

  onSubmit(event: Event): void {
    event.preventDefault();
    const metadata: Record<string, string> = {};
    for (const row of this.metadataRows()) {
      const key = row.form.key().value().trim();
      if (key) {
        metadata[key] = row.form.value().value();
      }
    }
    this.enqueue.emit({
      topic: this.enqueueForm.topic().value(),
      payload: this.enqueueForm.payload().value(),
      metadata,
    });
  }
}
