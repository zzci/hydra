import { prng } from "../../helpers"

describe("The OAuth 2.0 Authorization Code Grant", function () {
  const nc = () => ({
    client_secret: prng(),
    scope: "offline_access openid",
    subject_type: "public",
    token_endpoint_auth_method: "client_secret_basic",
    redirect_uris: [`${Cypress.env("client_url")}/oauth2/callback`],
    grant_types: ["authorization_code", "refresh_token"],
  })

  it("should return an Access, Refresh, and ID Token when scope offline_access and openid are granted", function () {
    const client = nc()
    cy.authCodeFlow(client, {
      consent: { scope: ["offline_access", "openid"] },
    })

    cy.get("body")
      .invoke("text")
      .then((content) => {
        const {
          result,
          token: { access_token, id_token, refresh_token },
        } = JSON.parse(content)

        expect(result).to.equal("success")
        expect(access_token).to.not.be.empty
        expect(id_token).to.not.be.empty
        expect(refresh_token).to.not.be.empty
      })
  })

  it("should return an Access and Refresh Token when scope offline_access is granted", function () {
    const client = nc()
    cy.authCodeFlow(client, { consent: { scope: ["offline_access"] } })

    cy.get("body")
      .invoke("text")
      .then((content) => {
        const {
          result,
          token: { access_token, id_token, refresh_token },
        } = JSON.parse(content)

        expect(result).to.equal("success")
        expect(access_token).to.not.be.empty
        expect(id_token).to.be.undefined
        expect(refresh_token).to.not.be.empty
      })
  })

  it("should return an Access and ID Token when scope offline_access is granted", function () {
    const client = nc()
    cy.authCodeFlow(client, { consent: { scope: ["openid"] } })

    cy.get("body")
      .invoke("text")
      .then((content) => {
        const {
          result,
          token: { access_token, id_token, refresh_token },
        } = JSON.parse(content)

        expect(result).to.equal("success")
        expect(access_token).to.not.be.empty
        expect(id_token).to.not.be.empty
        expect(refresh_token).to.be.undefined
      })
  })

  it("should return an Access Token when no scope is granted", function () {
    const client = nc()
    cy.authCodeFlow(client, { consent: { scope: [] } })

    cy.get("body")
      .invoke("text")
      .then((content) => {
        const {
          result,
          token: { access_token, id_token, refresh_token },
        } = JSON.parse(content)

        expect(result).to.equal("success")
        expect(access_token).to.not.be.empty
        expect(id_token).to.be.undefined
        expect(refresh_token).to.be.undefined
      })
  })
})
