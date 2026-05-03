import { Component, inject, signal, ChangeDetectionStrategy, OnInit } from '@angular/core';
import { SlicePipe } from '@angular/common';
import { UserService, User, Grant } from '../services/user.service';
import { getErrorMessage } from '../utils/error';
import { inputValue } from '../utils/dom';

@Component({
  selector: 'app-users-section',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [SlicePipe],
  template: `
    <section class="bg-white shadow rounded-lg">
      <div class="px-6 py-4 border-b border-gray-200">
        <div class="flex items-center justify-between">
          <h2 class="flex items-center gap-2 text-lg font-semibold text-gray-900">
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
                d="M15 19.128a9.38 9.38 0 0 0 2.625.372 9.337 9.337 0 0 0 4.121-.952 4.125 4.125 0 0 0-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 0 1 8.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0 1 11.964-3.07M12 6.375a3.375 3.375 0 1 1-6.75 0 3.375 3.375 0 0 1 6.75 0Zm8.25 2.25a2.625 2.625 0 1 1-5.25 0 2.625 2.625 0 0 1 5.25 0Z"
              />
            </svg>
            Users
          </h2>
          <button
            type="button"
            (click)="onAddNew()"
            [disabled]="addingNew()"
            class="flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-indigo-600 border border-indigo-300 rounded-md hover:bg-indigo-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke-width="1.5" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
            </svg>
            Add User
          </button>
        </div>
      </div>

      <div class="px-6 py-4">
        @if (error()) {
          <div class="mb-4 p-3 bg-red-50 border border-red-200 text-red-700 rounded text-sm">
            {{ error() }}
          </div>
        }

        @if (loading()) {
          <p class="text-sm text-gray-500">Loading…</p>
        } @else if (users().length === 0 && !addingNew()) {
          <p class="text-sm text-gray-500">No users found.</p>
        } @else {
          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Username</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Admin</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Created</th>
                  <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100">
                @if (addingNew()) {
                  <tr class="bg-indigo-50">
                    <td class="px-3 py-2">
                      <input
                        type="text"
                        [value]="newUsername()"
                        (input)="newUsername.set(inputValue($event))"
                        placeholder="username"
                        aria-label="New username"
                        class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                      />
                    </td>
                    <td class="px-3 py-2">
                      <input
                        type="password"
                        [value]="newPassword()"
                        (input)="newPassword.set(inputValue($event))"
                        placeholder="password"
                        aria-label="New password"
                        class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                      />
                    </td>
                    <td class="px-3 py-2">
                      <label class="flex items-center gap-2 cursor-pointer">
                        <input
                          type="checkbox"
                          [checked]="newIsAdmin()"
                          (change)="newIsAdmin.set(inputChecked($event))"
                          aria-label="Is admin"
                          class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-500"
                        />
                        <span class="text-sm text-gray-700">Admin</span>
                      </label>
                    </td>
                    <td class="px-3 py-2 flex items-center gap-2">
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
                    </td>
                  </tr>
                }
                @for (user of users(); track user.id) {
                  @if (editingUserId() === user.id) {
                    <tr class="bg-indigo-50">
                      <td class="px-3 py-2">
                        <input
                          type="text"
                          [value]="editForm().username"
                          (input)="patchEditForm('username', $any($event.target).value)"
                          placeholder="username"
                          [attr.aria-label]="'Edit username for ' + user.username"
                          class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                        />
                      </td>
                      <td class="px-3 py-2">
                        <input
                          type="password"
                          [value]="editForm().password"
                          (input)="patchEditForm('password', $any($event.target).value)"
                          placeholder="leave blank to keep"
                          [attr.aria-label]="'New password for ' + user.username"
                          class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                        />
                      </td>
                      <td class="px-3 py-2">
                        <label class="flex items-center gap-2 cursor-pointer">
                          <input
                            type="checkbox"
                            [checked]="editForm().is_admin"
                            (change)="patchEditFormBool('is_admin', $any($event.target).checked)"
                            [attr.aria-label]="'Is admin for ' + user.username"
                            class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-500"
                          />
                          <span class="text-sm text-gray-700">Admin</span>
                        </label>
                      </td>
                      <td class="px-3 py-2 flex items-center gap-2">
                        <button
                          type="button"
                          (click)="onSave(user.id)"
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
                      </td>
                    </tr>
                  } @else {
                    <tr class="hover:bg-gray-50">
                      <td class="px-3 py-2 font-medium text-gray-900">{{ user.username }}</td>
                      <td class="px-3 py-2">
                        @if (user.is_admin) {
                          <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-indigo-100 text-indigo-800">
                            Admin
                          </span>
                        } @else {
                          <span class="text-gray-400">—</span>
                        }
                      </td>
                      <td class="px-3 py-2 text-gray-500">{{ user.created_at | slice: 0 : 10 }}</td>
                      <td class="px-3 py-2 flex items-center gap-2">
                        <button
                          type="button"
                          (click)="toggleGrants(user.id)"
                          [attr.aria-label]="'Toggle grants for ' + user.username"
                          class="px-3 py-1 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
                        >
                          Grants
                        </button>
                        <button
                          type="button"
                          (click)="onEdit(user)"
                          [attr.aria-label]="'Edit ' + user.username"
                          class="px-3 py-1 text-sm font-medium text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-gray-400 cursor-pointer"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          (click)="onDelete(user.id)"
                          [attr.aria-label]="'Delete ' + user.username"
                          class="px-2 py-1 text-lg text-gray-400 hover:text-red-600 focus:outline-none cursor-pointer"
                        >
                          &times;
                        </button>
                      </td>
                    </tr>
                  }
                  @if (expandedGrantUserId() === user.id) {
                    <tr>
                      <td colspan="4" class="px-6 py-3 bg-gray-50">
                        <div class="border border-gray-200 rounded-md overflow-hidden">
                          <table class="min-w-full text-sm">
                            <thead class="bg-gray-100">
                              <tr>
                                <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Action</th>
                                <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Topic Pattern</th>
                                <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
                              </tr>
                            </thead>
                            <tbody class="divide-y divide-gray-100 bg-white">
                              @for (grant of grantsFor(user.id); track grant.id) {
                                <tr class="hover:bg-gray-50">
                                  <td class="px-3 py-2">{{ grant.action }}</td>
                                  <td class="px-3 py-2 font-mono text-xs">{{ grant.topic_pattern }}</td>
                                  <td class="px-3 py-2">
                                    <button
                                      type="button"
                                      (click)="onDeleteGrant(user.id, grant.id)"
                                      [attr.aria-label]="'Delete grant ' + grant.id"
                                      class="px-2 py-1 text-lg text-gray-400 hover:text-red-600 focus:outline-none cursor-pointer"
                                    >
                                      &times;
                                    </button>
                                  </td>
                                </tr>
                              }
                              <tr class="bg-indigo-50">
                                <td class="px-3 py-2">
                                  <select
                                    [value]="newGrantAction()"
                                    (change)="setNewGrantAction($event)"
                                    [attr.aria-label]="'Grant action for ' + user.username"
                                    class="w-full px-2 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                                  >
                                    <option value="read">read</option>
                                    <option value="write">write</option>
                                    <option value="admin">admin</option>
                                  </select>
                                </td>
                                <td class="px-3 py-2">
                                  <input
                                    type="text"
                                    [value]="newGrantPattern()"
                                    (input)="newGrantPattern.set(inputValue($event))"
                                    placeholder="topic pattern"
                                    [attr.aria-label]="'Grant topic pattern for ' + user.username"
                                    class="w-full px-3 py-1.5 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-indigo-500"
                                  />
                                </td>
                                <td class="px-3 py-2">
                                  <button
                                    type="button"
                                    (click)="onAddGrant(user.id)"
                                    [attr.aria-label]="'Add grant for ' + user.username"
                                    class="px-3 py-1 text-sm font-medium text-white bg-indigo-600 rounded-md hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-indigo-500 cursor-pointer"
                                  >
                                    Add
                                  </button>
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>
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
export class UsersSection implements OnInit {
  private readonly userSvc = inject(UserService);

  readonly users = signal<User[]>([]);
  readonly loading = signal(false);
  readonly error = signal('');
  readonly editingUserId = signal<string | null>(null);
  readonly editForm = signal({ username: '', password: '', is_admin: false });
  readonly addingNew = signal(false);
  readonly expandedGrantUserId = signal<string | null>(null);
  readonly grants = signal<Record<string, Grant[]>>({});

  readonly newUsername = signal('');
  readonly newPassword = signal('');
  readonly newIsAdmin = signal(false);
  readonly newGrantAction = signal<'read' | 'write' | 'admin'>('read');
  readonly newGrantPattern = signal('*');

  protected readonly inputValue = inputValue;

  grantsFor(userId: string): Grant[] {
    return this.grants()[userId] ?? [];
  }

  inputChecked(e: Event): boolean {
    return (e.target as HTMLInputElement).checked;
  }

  setNewGrantAction(e: Event): void {
    this.newGrantAction.set((e.target as HTMLSelectElement).value as 'read' | 'write' | 'admin');
  }

  ngOnInit(): void {
    this.loadUsers();
  }

  loadUsers(): void {
    this.loading.set(true);
    this.error.set('');
    this.userSvc.listUsers().subscribe({
      next: (users) => {
        this.users.set(users);
        this.loading.set(false);
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to load users'));
        this.loading.set(false);
      },
    });
  }

  onEdit(user: User): void {
    this.editForm.set({ username: user.username, password: '', is_admin: user.is_admin });
    this.editingUserId.set(user.id);
  }

  onCancelEdit(): void {
    this.editingUserId.set(null);
  }

  patchEditForm(field: 'username' | 'password', value: string): void {
    this.editForm.update((f) => ({ ...f, [field]: value }));
  }

  patchEditFormBool(field: 'is_admin', value: boolean): void {
    this.editForm.update((f) => ({ ...f, [field]: value }));
  }

  onSave(id: string): void {
    const f = this.editForm();
    const req: { username?: string; password?: string; is_admin?: boolean } = {
      username: f.username || undefined,
      is_admin: f.is_admin,
    };
    if (f.password) {
      req['password'] = f.password;
    }
    this.userSvc.updateUser(id, req).subscribe({
      next: (updated) => {
        this.users.update((list) => list.map((u) => (u.id === id ? updated : u)));
        this.editingUserId.set(null);
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to update user'));
      },
    });
  }

  onDelete(id: string): void {
    this.userSvc.deleteUser(id).subscribe({
      next: () => {
        this.users.update((list) => list.filter((u) => u.id !== id));
        if (this.expandedGrantUserId() === id) {
          this.expandedGrantUserId.set(null);
        }
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to delete user'));
      },
    });
  }

  onAddNew(): void {
    this.newUsername.set('');
    this.newPassword.set('');
    this.newIsAdmin.set(false);
    this.addingNew.set(true);
  }

  onCancelNew(): void {
    this.addingNew.set(false);
  }

  onSaveNew(): void {
    if (!this.newUsername().trim()) {
      this.error.set('Username is required');
      return;
    }
    this.userSvc
      .createUser({
        username: this.newUsername().trim(),
        password: this.newPassword(),
        is_admin: this.newIsAdmin(),
      })
      .subscribe({
        next: (created) => {
          this.users.update((list) => [...list, created]);
          this.addingNew.set(false);
          this.newUsername.set('');
          this.newPassword.set('');
          this.newIsAdmin.set(false);
        },
        error: (err: unknown) => {
          this.error.set(getErrorMessage(err, 'Failed to create user'));
        },
      });
  }

  toggleGrants(userId: string): void {
    if (this.expandedGrantUserId() === userId) {
      this.expandedGrantUserId.set(null);
      return;
    }
    this.userSvc.listGrants(userId).subscribe({
      next: (grantList) => {
        this.grants.update((all) => ({ ...all, [userId]: grantList }));
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to load grants'));
      },
    });
    this.expandedGrantUserId.set(userId);
  }

  onDeleteGrant(userId: string, grantId: string): void {
    this.userSvc.deleteGrant(userId, grantId).subscribe({
      next: () => {
        this.grants.update((all) => ({
          ...all,
          [userId]: (all[userId] ?? []).filter((g) => g.id !== grantId),
        }));
      },
      error: (err: unknown) => {
        this.error.set(getErrorMessage(err, 'Failed to delete grant'));
      },
    });
  }

  onAddGrant(userId: string): void {
    this.userSvc
      .addGrant(userId, {
        action: this.newGrantAction(),
        topic_pattern: this.newGrantPattern() || '*',
      })
      .subscribe({
        next: (grant) => {
          this.grants.update((all) => ({
            ...all,
            [userId]: [...(all[userId] ?? []), grant],
          }));
          this.newGrantAction.set('read');
          this.newGrantPattern.set('*');
        },
        error: (err: unknown) => {
          this.error.set(getErrorMessage(err, 'Failed to add grant'));
        },
      });
  }
}
