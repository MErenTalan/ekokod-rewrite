-- Billing parameters (migration 00014): platform-wide, dated (R106). Enum
-- arrays travel as text[]: pgx has no codec for arrays of custom enums.

-- name: BillingParametersEffective :one
select effective_from, reactive_penalty_basis, reactive_exempt_below_kw,
  reactive_exempt_terms::text[] as reactive_exempt_terms,
  reactive_exempt_user_groups::text[] as reactive_exempt_user_groups,
  reactive_generation_exempt_kwh, reactive_bands, tiering_groups, tiering_mode,
  tiering_voltage_levels::text[] as tiering_voltage_levels,
  tiering_supply_companies::text[] as tiering_supply_companies,
  ptf_missing_hour_tolerance, demand_overrun_multiplier, money_rounding_mode, created_at
from billing_parameters
where effective_from <= sqlc.arg(effective_on)
order by effective_from desc
limit 1;

-- name: BillingParametersList :many
select effective_from, reactive_penalty_basis, reactive_exempt_below_kw,
  reactive_exempt_terms::text[] as reactive_exempt_terms,
  reactive_exempt_user_groups::text[] as reactive_exempt_user_groups,
  reactive_generation_exempt_kwh, reactive_bands, tiering_groups, tiering_mode,
  tiering_voltage_levels::text[] as tiering_voltage_levels,
  tiering_supply_companies::text[] as tiering_supply_companies,
  ptf_missing_hour_tolerance, demand_overrun_multiplier, money_rounding_mode, created_at
from billing_parameters
order by effective_from;
