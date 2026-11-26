import type { HandleClientError } from '@sveltejs/kit/hooks';

export const handleError: HandleClientError = ({ error, kind }) => {
  console.error('[Kinetiq client error]', kind, error);
  if (kind === 'app') return;
  return { message: 'Something went wrong. Please try again.' };
};
