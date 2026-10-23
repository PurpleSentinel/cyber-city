// SPDX-License-Identifier: MIT
import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist/", "node_modules/", "test-results/", "playwright-report/"] },
  js.configs.recommended,
  ...tseslint.configs.strict,
  {
    languageOptions: { globals: { ...globals.browser } },
    rules: {
      "@typescript-eslint/no-explicit-any": "error",
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_", ignoreRestSiblings: true },
      ],
    },
  },
  {
    // The city is solid, lit and shadowed: never wireframe (docs/DECISIONS.md
    // D17, TESTING.md "Shading guards"). The searchlight cone and target
    // outlines and the meteor shield are unlit overlays, so they are exempt.
    files: ["src/render/**/*.ts"],
    ignores: ["src/render/effects/meteor.ts", "src/render/effects/searchlight.ts"],
    rules: {
      "no-restricted-syntax": [
        "error",
        { selector: "Property[key.name='wireframe'][value.value=true]", message: "City geometry must not be wireframe (D17)." },
        { selector: "NewExpression[callee.name='MeshBasicMaterial'], NewExpression[callee.property.name='MeshBasicMaterial']", message: "Use a lit MeshStandardMaterial for city geometry (D17)." },
        { selector: "NewExpression[callee.name='EdgesGeometry'], NewExpression[callee.property.name='EdgesGeometry']", message: "Edge geometry is not a building body (D17)." },
        { selector: "NewExpression[callee.name='WireframeGeometry'], NewExpression[callee.property.name='WireframeGeometry']", message: "Wireframe geometry is not allowed for city geometry (D17)." },
      ],
    },
  },
  {
    // Tests may assert presence with `!`: a wrong guess fails the test anyway.
    files: ["test/**/*.ts", "e2e/**/*.ts"],
    rules: { "@typescript-eslint/no-non-null-assertion": "off" },
  },
);
