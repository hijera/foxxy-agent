// Steps for the desktop guided tour. Each step spotlights a live DOM element
// (found by `anchor`, a CSS selector) and shows an i18n title/body bubble.
//
// `optional` steps are silently skipped when their anchor is not on the page
// (e.g. the scheduler button is absent when the binary lacks scheduler routes,
// and the project pill only renders once a project is open).

export type TourStep = {
  /** Stable id; also drives the i18n keys `tour.step.<id>.title|body`. */
  id: string;
  /** CSS selector (may be a comma list) resolved via querySelector at runtime. */
  anchor: string;
  titleKey: string;
  bodyKey: string;
  optional?: boolean;
};

export const TOUR_STEPS: TourStep[] = [
  {
    id: "settings",
    anchor: '[data-testid="nav-settings"]',
    titleKey: "tour.step.settings.title",
    bodyKey: "tour.step.settings.body",
  },
  {
    id: "project",
    anchor: '[data-testid="project-pill-hero"], [data-testid="project-pill"]',
    titleKey: "tour.step.project.title",
    bodyKey: "tour.step.project.body",
    optional: true,
  },
  {
    id: "history",
    anchor: '[data-testid="nav-history"]',
    titleKey: "tour.step.history.title",
    bodyKey: "tour.step.history.body",
  },
  {
    id: "scheduler",
    anchor: '[data-testid="nav-scheduler"]',
    titleKey: "tour.step.scheduler.title",
    bodyKey: "tour.step.scheduler.body",
    optional: true,
  },
  {
    id: "model",
    anchor: '[data-testid="composer-model-btn"]',
    titleKey: "tour.step.model.title",
    bodyKey: "tour.step.model.body",
    optional: true,
  },
  {
    id: "input",
    anchor: "#composer",
    titleKey: "tour.step.input.title",
    bodyKey: "tour.step.input.body",
  },
  {
    id: "send",
    anchor: "#btn-send",
    titleKey: "tour.step.send.title",
    bodyKey: "tour.step.send.body",
  },
];
