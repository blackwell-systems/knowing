import {makeProject} from '@motion-canvas/core';
import identityIsContent from './scenes/identity-is-content?scene';
import graphProducesContext from './scenes/graph-produces-context?scene';

export default makeProject({
  scenes: [graphProducesContext],
});
