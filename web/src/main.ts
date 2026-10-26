import { mount } from 'svelte';
import App from './App.svelte';
import { restoreDestination, tabStorage } from './lib/signin';
import './theme.css';

const target = document.getElementById('app');
if (!target) throw new Error('missing #app');

// Back from signing in again: put back what the sign-in lost of the address.
const restored = restoreDestination(window.location, tabStorage());
if (restored !== undefined) history.replaceState(null, '', restored);

mount(App, { target });
