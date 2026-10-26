// Entry of fixtures.html, which `npm run dev` serves and `npm run build` leaves out.
import { mount } from 'svelte';
import '../theme.css';
import FixturePreview from './FixturePreview.svelte';

const target = document.getElementById('app');
if (!target) throw new Error('missing #app');

mount(FixturePreview, { target });
