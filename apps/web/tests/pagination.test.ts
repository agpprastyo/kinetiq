import { describe, expect, test } from "bun:test";
import { itemsOf } from "../src/lib/api";

describe("itemsOf", () => {
  test("unwraps {items, pagination}", () => {
    expect(itemsOf({ items: [1, 2], pagination: { page: 1, limit: 20, total: 2, total_pages: 1 } })).toEqual([1, 2]);
  });
  test("passes legacy bare arrays through", () => {
    expect(itemsOf([1, 2, 3])).toEqual([1, 2, 3]);
  });
  test("empty for null/undefined", () => {
    expect(itemsOf(null)).toEqual([]);
    expect(itemsOf(undefined)).toEqual([]);
  });
});
