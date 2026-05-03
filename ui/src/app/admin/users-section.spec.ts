import { TestBed } from '@angular/core/testing';
import { provideZonelessChangeDetection } from '@angular/core';
import { of, throwError } from 'rxjs';
import { vi } from 'vitest';
import { UsersSection } from './users-section';
import { UserService, User, Grant } from '../services/user.service';

const makeUser = (overrides: Partial<User> = {}): User => ({
  id: 'user-1',
  username: 'alice',
  is_admin: false,
  created_at: '2024-01-15T10:00:00Z',
  updated_at: '2024-01-15T10:00:00Z',
  ...overrides,
});

const makeGrant = (overrides: Partial<Grant> = {}): Grant => ({
  id: 'grant-1',
  user_id: 'user-1',
  action: 'read',
  topic_pattern: '*',
  created_at: '2024-01-15T10:00:00Z',
  ...overrides,
});

const makeUserService = (opts: {
  listResult?: User[] | 'error';
  createResult?: User | 'error';
  updateResult?: User | 'error';
  deleteResult?: 'ok' | 'error';
  listGrantsResult?: Grant[] | 'error';
  addGrantResult?: Grant | 'error';
  deleteGrantResult?: 'ok' | 'error';
  addConsumerGroupGrantResult?: Grant | 'error';
} = {}) => {
  const {
    listResult = [],
    createResult = makeUser(),
    updateResult = makeUser(),
    deleteResult = 'ok',
    listGrantsResult = [],
    addGrantResult = makeGrant(),
    deleteGrantResult = 'ok',
    addConsumerGroupGrantResult = makeGrant({ action: 'consume', consumer_group: 'default' }),
  } = opts;

  return {
    listUsers: vi.fn().mockReturnValue(
      listResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to load users' } }))
        : of(listResult as User[]),
    ),
    createUser: vi.fn().mockReturnValue(
      createResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to create user' } }))
        : of(createResult as User),
    ),
    updateUser: vi.fn().mockReturnValue(
      updateResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to update user' } }))
        : of(updateResult as User),
    ),
    deleteUser: vi.fn().mockReturnValue(
      deleteResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to delete user' } }))
        : of(undefined),
    ),
    listGrants: vi.fn().mockReturnValue(
      listGrantsResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to load grants' } }))
        : of(listGrantsResult as Grant[]),
    ),
    addGrant: vi.fn().mockReturnValue(
      addGrantResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to add grant' } }))
        : of(addGrantResult as Grant),
    ),
    deleteGrant: vi.fn().mockReturnValue(
      deleteGrantResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to delete grant' } }))
        : of(undefined),
    ),
    addConsumerGroupGrant: vi.fn().mockReturnValue(
      addConsumerGroupGrantResult === 'error'
        ? throwError(() => ({ error: { error: 'Failed to add consumer group grant' } }))
        : of(addConsumerGroupGrantResult as Grant),
    ),
  } as unknown as UserService;
};

const setup = async (opts: {
  users?: User[];
  loadError?: boolean;
  createResult?: User | 'error';
  updateResult?: User | 'error';
  deleteResult?: 'ok' | 'error';
  listGrantsResult?: Grant[] | 'error';
  addGrantResult?: Grant | 'error';
  deleteGrantResult?: 'ok' | 'error';
  addConsumerGroupGrantResult?: Grant | 'error';
} = {}) => {
  const {
    users = [],
    loadError = false,
    createResult,
    updateResult,
    deleteResult,
    listGrantsResult,
    addGrantResult,
    deleteGrantResult,
    addConsumerGroupGrantResult,
  } = opts;

  const userService = makeUserService({
    listResult: loadError ? 'error' : users,
    createResult: createResult ?? makeUser(),
    updateResult: updateResult ?? makeUser(),
    deleteResult: deleteResult ?? 'ok',
    listGrantsResult: listGrantsResult ?? [],
    addGrantResult: addGrantResult ?? makeGrant(),
    deleteGrantResult: deleteGrantResult ?? 'ok',
    addConsumerGroupGrantResult: addConsumerGroupGrantResult ?? makeGrant({ action: 'consume', consumer_group: 'default' }),
  });

  await TestBed.configureTestingModule({
    imports: [UsersSection],
    providers: [
      provideZonelessChangeDetection(),
      { provide: UserService, useValue: userService },
    ],
  }).compileComponents();

  const fixture = TestBed.createComponent(UsersSection);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();

  const el: HTMLElement = fixture.nativeElement;
  return { fixture, component: fixture.componentInstance, userService, el };
};

describe('UsersSection', () => {
  describe('on init', () => {
    it('should call listUsers()', async () => {
      const { userService } = await setup();
      expect(userService.listUsers).toHaveBeenCalledOnce();
    });
  });

  describe('when users are returned', () => {
    it('should render a row for each user', async () => {
      const users = [
        makeUser({ id: 'u1', username: 'alice' }),
        makeUser({ id: 'u2', username: 'bob' }),
        makeUser({ id: 'u3', username: 'carol' }),
      ];
      const { el } = await setup({ users });

      const rows = el.querySelectorAll('tbody > tr');
      expect(rows.length).toBe(3);
    });

    it('should show admin badge for admin users', async () => {
      const users = [makeUser({ id: 'u1', username: 'admin', is_admin: true })];
      const { el } = await setup({ users });

      const badge = el.querySelector('span.bg-indigo-100');
      expect(badge).not.toBeNull();
      expect(badge?.textContent?.trim()).toBe('Admin');
    });

    it('should not show admin badge for non-admin users', async () => {
      const users = [makeUser({ id: 'u1', username: 'alice', is_admin: false })];
      const { el } = await setup({ users });

      const badge = el.querySelector('span.bg-indigo-100');
      expect(badge).toBeNull();
    });
  });

  describe('when loading fails', () => {
    it('should display an error message', async () => {
      const { el } = await setup({ loadError: true });
      expect(el.textContent).toContain('Failed to load users');
    });
  });

  describe('when adding a new user', () => {
    it('should expand the add-user form', async () => {
      const { fixture, el, component } = await setup();

      const addBtn = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.trim().includes('Add User'),
      ) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.addingNew()).toBe(true);
      expect(el.querySelector('input[aria-label="New username"]')).not.toBeNull();
    });

    it('should call createUser() on save', async () => {
      const newUser = makeUser({ id: 'u-new', username: 'newuser' });
      const { fixture, el, component, userService } = await setup({
        createResult: newUser,
      });

      const addBtn = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.trim().includes('Add User'),
      ) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.newUsername.set('newuser');
      component.newPassword.set('pass123');
      component.newIsAdmin.set(false);

      const saveBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Save',
      ) as HTMLButtonElement;
      saveBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(userService.createUser).toHaveBeenCalledWith({
        username: 'newuser',
        password: 'pass123',
        is_admin: false,
      });
    });

    it('should add the new user to the list on success', async () => {
      const newUser = makeUser({ id: 'u-new', username: 'newuser' });
      const { fixture, el, component } = await setup({ createResult: newUser });

      const addBtn = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.trim().includes('Add User'),
      ) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.newUsername.set('newuser');
      component.newPassword.set('pass123');

      const saveBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Save',
      ) as HTMLButtonElement;
      saveBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.users().length).toBe(1);
      expect(component.users()[0].username).toBe('newuser');
      expect(component.addingNew()).toBe(false);
    });

    it('should cancel and hide the form on cancel', async () => {
      const { fixture, el, component } = await setup();

      const addBtn = Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.trim().includes('Add User'),
      ) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const cancelBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Cancel',
      ) as HTMLButtonElement;
      cancelBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.addingNew()).toBe(false);
      expect(el.querySelector('input[aria-label="New username"]')).toBeNull();
    });
  });

  describe('when editing a user', () => {
    it('should call updateUser() on save', async () => {
      const user = makeUser({ id: 'u1', username: 'alice', is_admin: false });
      const updated = makeUser({ id: 'u1', username: 'alice-updated', is_admin: false });
      const { fixture, el, component, userService } = await setup({
        users: [user],
        updateResult: updated,
      });

      const editBtn = el.querySelector(`button[aria-label="Edit alice"]`) as HTMLButtonElement;
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.patchEditForm('username', 'alice-updated');
      fixture.detectChanges();

      const saveBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Save',
      ) as HTMLButtonElement;
      saveBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(userService.updateUser).toHaveBeenCalledWith(
        'u1',
        expect.objectContaining({ username: 'alice-updated' }),
      );
    });

    it('should update the user in the list on success', async () => {
      const user = makeUser({ id: 'u1', username: 'alice', is_admin: false });
      const updated = makeUser({ id: 'u1', username: 'alice-updated', is_admin: false });
      const { fixture, el, component } = await setup({ users: [user], updateResult: updated });

      const editBtn = el.querySelector(`button[aria-label="Edit alice"]`) as HTMLButtonElement;
      editBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const saveBtn = Array.from(el.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Save',
      ) as HTMLButtonElement;
      saveBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.users()[0].username).toBe('alice-updated');
      expect(component.editingUserId()).toBeNull();
    });
  });

  describe('when deleting a user', () => {
    it('should call deleteUser()', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const { el, userService } = await setup({ users: [user] });

      const deleteBtn = el.querySelector(`button[aria-label="Delete alice"]`) as HTMLButtonElement;
      deleteBtn.click();

      expect(userService.deleteUser).toHaveBeenCalledWith('u1');
    });

    it('should remove the user from the list on success', async () => {
      const users = [
        makeUser({ id: 'u1', username: 'alice' }),
        makeUser({ id: 'u2', username: 'bob' }),
      ];
      const { fixture, el, component } = await setup({ users });

      const deleteBtn = el.querySelector(`button[aria-label="Delete alice"]`) as HTMLButtonElement;
      deleteBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.users().length).toBe(1);
      expect(component.users()[0].username).toBe('bob');
    });
  });

  describe('when toggling grants', () => {
    it('should call listGrants() and show grants', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const grants = [makeGrant({ id: 'g1', user_id: 'u1', action: 'read', topic_pattern: 'orders.*' })];
      const { fixture, el, component, userService } = await setup({
        users: [user],
        listGrantsResult: grants,
      });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(userService.listGrants).toHaveBeenCalledWith('u1');
      expect(component.expandedGrantUserId()).toBe('u1');
      expect(component.grants()['u1']).toHaveLength(1);
    });

    it('should collapse on second toggle', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const { fixture, el, component } = await setup({ users: [user] });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;

      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.expandedGrantUserId()).toBeNull();
    });
  });

  describe('when adding a grant', () => {
    it('should call addGrant() and update the grants list', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const newGrant = makeGrant({ id: 'g1', user_id: 'u1', action: 'write', topic_pattern: 'orders' });
      const { fixture, el, component, userService } = await setup({
        users: [user],
        listGrantsResult: [],
        addGrantResult: newGrant,
      });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.newGrantAction.set('write');
      component.newGrantPattern.set('orders');

      const addBtn = el.querySelector(`button[aria-label="Add grant for alice"]`) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(userService.addGrant).toHaveBeenCalledWith('u1', {
        action: 'write',
        topic_pattern: 'orders',
      });
      expect(component.grants()['u1']).toHaveLength(1);
      expect(component.grants()['u1'][0].action).toBe('write');
    });
  });

  describe('when deleting a grant', () => {
    it('should call deleteGrant() and remove the grant from the list', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const grants = [
        makeGrant({ id: 'g1', user_id: 'u1' }),
        makeGrant({ id: 'g2', user_id: 'u1', action: 'write' }),
      ];
      const { fixture, el, component, userService } = await setup({
        users: [user],
        listGrantsResult: grants,
      });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const deleteGrantBtn = el.querySelector(`button[aria-label="Delete grant g1"]`) as HTMLButtonElement;
      deleteGrantBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(userService.deleteGrant).toHaveBeenCalledWith('u1', 'g1');
      expect(component.grants()['u1']).toHaveLength(1);
      expect(component.grants()['u1'][0].id).toBe('g2');
    });
  });

  describe('grants table Consumer Group column', () => {
    it('should render the Consumer Group column header', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const { fixture, el } = await setup({ users: [user] });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const headers = Array.from(el.querySelectorAll('th')).map((th) => th.textContent?.trim());
      expect(headers).toContain('Consumer Group');
    });

    it('should render — for grants without a consumer_group', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const grants = [makeGrant({ id: 'g1', user_id: 'u1', action: 'read', topic_pattern: '*' })];
      const { fixture, el } = await setup({ users: [user], listGrantsResult: grants });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const rows = el.querySelectorAll('tbody tr');
      // first data row (not the add-grant form row)
      const firstRow = rows[0];
      const cells = firstRow.querySelectorAll('td');
      expect(cells[2].textContent?.trim()).toBe('—');
    });

    it('should render the consumer_group value for consume grants', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const grants = [
        makeGrant({
          id: 'g1',
          user_id: 'u1',
          action: 'consume',
          topic_pattern: 'orders.*',
          consumer_group: 'my-group',
        }),
      ];
      const { fixture, el } = await setup({ users: [user], listGrantsResult: grants });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      const firstRow = el.querySelectorAll('tbody tr')[0];
      const cells = firstRow.querySelectorAll('td');
      expect(cells[2].textContent?.trim()).toBe('my-group');
    });
  });

  describe('when adding a consumer group grant', () => {
    it('should call addConsumerGroupGrant() with the entered values', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const newGrant = makeGrant({
        id: 'g-cg',
        user_id: 'u1',
        action: 'consume',
        topic_pattern: 'orders.*',
        consumer_group: 'workers',
      });
      const { fixture, el, component, userService } = await setup({
        users: [user],
        listGrantsResult: [],
        addConsumerGroupGrantResult: newGrant,
      });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.newCgPattern.set('orders.*');
      component.newCgGroup.set('workers');
      fixture.detectChanges();

      const addBtn = el.querySelector(`button[aria-label="Add consumer group grant for alice"]`) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(userService.addConsumerGroupGrant).toHaveBeenCalledWith('u1', 'orders.*', 'workers');
      expect(component.grants()['u1']).toHaveLength(1);
      expect(component.grants()['u1'][0].consumer_group).toBe('workers');
    });

    it('should reset the form fields after a successful submission', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const { fixture, el, component } = await setup({ users: [user], listGrantsResult: [] });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.newCgPattern.set('events.*');
      component.newCgGroup.set('my-group');

      const addBtn = el.querySelector(`button[aria-label="Add consumer group grant for alice"]`) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.newCgPattern()).toBe('*');
      expect(component.newCgGroup()).toBe('');
    });

    it('should show an error banner when consumer group is empty', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const { fixture, el, component } = await setup({ users: [user], listGrantsResult: [] });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.newCgGroup.set('');
      fixture.detectChanges();

      const addBtn = el.querySelector(`button[aria-label="Add consumer group grant for alice"]`) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.grantsError('u1')).toBe('Consumer group is required');
      expect(el.textContent).toContain('Consumer group is required');
    });

    it('should show an error banner when the request fails', async () => {
      const user = makeUser({ id: 'u1', username: 'alice' });
      const { fixture, el, component } = await setup({
        users: [user],
        listGrantsResult: [],
        addConsumerGroupGrantResult: 'error',
      });

      const grantsBtn = el.querySelector(`button[aria-label="Toggle grants for alice"]`) as HTMLButtonElement;
      grantsBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      component.newCgGroup.set('workers');
      fixture.detectChanges();

      const addBtn = el.querySelector(`button[aria-label="Add consumer group grant for alice"]`) as HTMLButtonElement;
      addBtn.click();
      await fixture.whenStable();
      fixture.detectChanges();

      expect(component.grantsError('u1')).toBe('Failed to add consumer group grant');
      expect(el.textContent).toContain('Failed to add consumer group grant');
    });
  });
});
