package contact

import (
	"billionmail-core/api/contact/v1"
	"billionmail-core/internal/service/contact"
	"billionmail-core/internal/service/public"
	"billionmail-core/internal/service/tenants"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/frame/g"

	"github.com/gogf/gf/v2/errors/gerror"
)

func (c *ControllerV1) GetSingleGroupTagContactCount(ctx context.Context, req *v1.GetSingleGroupTagContactCountReq) (res *v1.GetSingleGroupTagContactCountRes, err error) {
	res = &v1.GetSingleGroupTagContactCountRes{}

	if len(req.TagIds) == 0 {
		GroupIds := []int{req.GroupId}
		total, err := contact.GetGroupContactCount(ctx, GroupIds)
		if err != nil {
			res.Code = 500
			res.SetError(gerror.New(public.LangCtx(ctx, "Failed to get contact count: {}", err.Error())))
			return res, nil
		}
		// Set response data
		res.Data.Total = total
		res.SetSuccess(public.LangCtx(ctx, "Successfully retrieved contact count"))
		return res, nil
	}

	tenantID, err := tenants.RequireTenantID(ctx)
	if err != nil {
		res.SetError(err)
		return
	}

	model := tenants.ScopeModel(ctx, g.DB().Model("bm_contacts c"), "c.tenant_id").
		Where("c.active", 1).
		Where("c.status", 1)

	if req.GroupId > 0 {
		model = model.Where("c.group_id", req.GroupId)
	}

	if len(req.TagIds) > 0 {
		if req.TagLogic == "AND" {
			// AND logic: contact must have all tags
			for i, tagId := range req.TagIds {
				alias := "ct" + strconv.Itoa(i)
				model = model.InnerJoin(
					"bm_contact_tags "+alias,
					fmt.Sprintf("c.id = %s.contact_id AND %s.tenant_id = %d AND %s.tag_id = %d", alias, alias, tenantID, alias, tagId),
				)
			}
		} else if req.TagLogic == "OR" {
			// OR logic: contact must have at least one of the tags
			var inValues []string
			for _, tagId := range req.TagIds {
				inValues = append(inValues, strconv.Itoa(tagId))
			}
			subQuery := fmt.Sprintf(
				"(SELECT DISTINCT contact_id FROM bm_contact_tags WHERE tenant_id = %d AND tag_id IN (%s)) ct",
				tenantID,
				strings.Join(inValues, ","),
			)
			model = model.InnerJoin(subQuery, "c.id = ct.contact_id")
		} else if req.TagLogic == "NOT" {
			// NOT logic: exclude contacts that have any of the specified tags
			var tagIdStr []string
			for _, tagId := range req.TagIds {
				tagIdStr = append(tagIdStr, strconv.Itoa(tagId))
			}

			subQuery := tenants.ScopeModel(ctx, g.DB().Model("bm_contact_tags"), "tenant_id").
				Fields("DISTINCT contact_id").
				WhereIn("tag_id", req.TagIds)
			model = model.WhereNotIn("c.id", subQuery)
		}
	}

	total, err := model.Fields("DISTINCT c.id").Count()

	if err != nil {
		res.SetError(gerror.New(public.LangCtx(ctx, "Failed to get contact count: {}", err.Error())))
		return
	}

	// Set response data
	res.Data.Total = total
	res.SetSuccess(public.LangCtx(ctx, "Successfully retrieved contact count"))
	return
}
